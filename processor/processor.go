package processor

import (
	"context"
	"fmt"
	"sync"

	"github.com/CodMac/go-treesitter-dependency-analyzer/extractor"
	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	"github.com/CodMac/go-treesitter-dependency-analyzer/parser"
)

// FileProcessor 负责管理文件解析和依赖提取的并发调度。
type FileProcessor struct {
	Language model.Language
	Workers  int // 并发协程数量
}

// NewFileProcessor 创建一个新的处理器实例。
func NewFileProcessor(lang model.Language, workers int) *FileProcessor {
	if workers <= 0 {
		workers = 4 // 默认并发数
	}
	return &FileProcessor{
		Language: lang,
		Workers:  workers,
	}
}

// ProcessFiles 实现了两阶段处理逻辑：收集定义 -> 提取关系。
func (fp *FileProcessor) ProcessFiles(ctx context.Context, filePaths []string) ([]*model.DependencyRelation, error) {
	if len(filePaths) == 0 {
		return nil, nil
	}

	// GlobalContext 是所有并发阶段共享和更新的中央符号表。
	globalContext := extractor.NewGlobalContext()

	// --- 阶段 1: 收集定义 (Definition Pass) ---
	// 目的：并发解析所有文件，并填充 globalContext 中的所有符号定义。
	fmt.Printf("Phase 1: Collecting definitions from %d files with %d workers...\n", len(filePaths), fp.Workers)
	if _, err := fp.runPhase(ctx, filePaths, globalContext, fp.workerPhase1); err != nil {
		return nil, fmt.Errorf("phase 1 (definition collection) failed: %w", err)
	}

	// --- 阶段 2: 提取关系 (Relation Pass) ---
	// 目的：利用 Phase 1 建立的 globalContext，并发提取依赖关系。
	fmt.Printf("Phase 2: Extracting dependencies...\n")
	results, err := fp.runPhase(ctx, filePaths, globalContext, fp.workerPhase2)
	if err != nil {
		return nil, fmt.Errorf("phase 2 (relation extraction) failed: %w", err)
	}

	// 聚合 Phase 2 的依赖关系结果
	var allRelations []*model.DependencyRelation
	if allRes, ok := results.([]interface{}); ok {
		for _, res := range allRes {
			// 每个 workerPhase2 返回的是 []*model.DependencyRelation
			if rels, ok := res.([]*model.DependencyRelation); ok {
				allRelations = append(allRelations, rels...)
			}
		}
	}

	return allRelations, nil
}

// workerFunc 定义了并发工作协程的签名。
type workerFunc func(context.Context, *sync.WaitGroup, <-chan string, chan interface{}, chan error, *extractor.GlobalContext)

// runPhase 调度并发工作协程并等待结果。
func (fp *FileProcessor) runPhase(ctx context.Context, filePaths []string, gc *extractor.GlobalContext, workerFn workerFunc) (interface{}, error) {
	filesChan := make(chan string, len(filePaths))
	// 结果通道用于收集 workerFn 返回的任意结果 (interface{})
	resultsChan := make(chan interface{}, len(filePaths))
	errChan := make(chan error, fp.Workers)
	var wg sync.WaitGroup

	// 启动 worker 协程
	for i := 0; i < fp.Workers; i++ {
		wg.Add(1)
		go workerFn(ctx, &wg, filesChan, resultsChan, errChan, gc)
	}

	// 填充文件路径到通道
	for _, path := range filePaths {
		filesChan <- path
	}
	close(filesChan)

	// 等待所有 worker 完成，然后关闭结果通道
	go func() {
		wg.Wait()
		close(resultsChan)
		close(errChan)
	}()

	var allResults []interface{}
	for res := range resultsChan {
		allResults = append(allResults, res)
	}

	// 检查是否有任何 worker 报告了错误
	if err := <-errChan; err != nil {
		return nil, err
	}

	return allResults, nil
}

// workerPhase1 负责执行 AST 解析和定义收集。
func (fp *FileProcessor) workerPhase1(ctx context.Context, wg *sync.WaitGroup, filesChan <-chan string, resultsChan chan interface{}, errChan chan error, gc *extractor.GlobalContext) {
	defer wg.Done()

	// 每个 worker 维护自己的 Parser 实例
	p, pErr := parser.NewParser(fp.Language)
	if pErr != nil {
		// 致命错误：无法创建解析器，退出所有 worker
		select {
		case errChan <- fmt.Errorf("failed to create parser for %s: %w", fp.Language, pErr):
		default:
		}
		return
	}
	defer p.Close()

	ext, extErr := extractor.GetExtractor(fp.Language)
	if extErr != nil {
		// 致命错误：无法获取 Extractor
		select {
		case errChan <- fmt.Errorf("failed to get extractor for %s: %w", fp.Language, extErr):
		default:
		}
		return
	}
	collector, ok := ext.(extractor.DefinitionCollector)
	if !ok {
		// 致命错误：Extractor 未实现 DefinitionCollector 接口
		select {
		case errChan <- fmt.Errorf("extractor for %s does not implement DefinitionCollector", fp.Language):
		default:
		}
		return
	}

	for filePath := range filesChan {
		rootNode, err := p.ParseFile(filePath)
		if err != nil {
			fmt.Printf("[Warning P1] Skipping %s due to parsing error: %v\n", filePath, err)
			continue
		}

		// 调用 Extractor 的定义收集方法
		fileContext, err := collector.CollectDefinitions(rootNode, filePath)
		if err != nil {
			fmt.Printf("[Warning P1] Failed to collect definitions in %s: %v\n", filePath, err)
			continue
		}

		// 将结果安全地注册到全局上下文 (GlobalContext 是并发安全的)
		gc.RegisterFileContext(fileContext)
	}
}

// workerPhase2 负责执行 AST 解析和依赖关系提取。
func (fp *FileProcessor) workerPhase2(ctx context.Context, wg *sync.WaitGroup, filesChan <-chan string, resultsChan chan interface{}, errChan chan error, gc *extractor.GlobalContext) {
	defer wg.Done()

	// 每个 worker 维护自己的 Parser 实例
	p, pErr := parser.NewParser(fp.Language)
	if pErr != nil {
		select {
		case errChan <- fmt.Errorf("failed to create parser for %s: %w", fp.Language, pErr):
		default:
		}
		return
	}
	defer p.Close()

	ext, extErr := extractor.GetExtractor(fp.Language)
	if extErr != nil {
		select {
		case errChan <- fmt.Errorf("failed to get extractor for %s: %w", fp.Language, extErr):
		default:
		}
		return
	}
	contextExt, ok := ext.(extractor.ContextExtractor)
	if !ok {
		select {
		case errChan <- fmt.Errorf("extractor for %s does not implement ContextExtractor", fp.Language):
		default:
		}
		return
	}

	for filePath := range filesChan {
		rootNode, err := p.ParseFile(filePath)
		if err != nil {
			// P1 阶段已经报告了致命的解析错误，这里跳过
			continue
		}

		// 调用 Extractor 的关系提取方法，传入完整的 GlobalContext
		relations, err := contextExt.Extract(rootNode, filePath, gc)
		if err != nil {
			fmt.Printf("[Warning P2] Failed to extract relations in %s: %v\n", filePath, err)
			continue
		}

		// 将提取到的依赖关系发送给结果通道
		resultsChan <- relations
	}
}
