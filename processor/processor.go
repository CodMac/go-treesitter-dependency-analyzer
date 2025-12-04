package processor

import (
	"context"
	"fmt"
	"sync"

	"github.com/CodMac/go-treesitter-dependency-analyzer/collector"
	"github.com/CodMac/go-treesitter-dependency-analyzer/extractor"
	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	"github.com/CodMac/go-treesitter-dependency-analyzer/parser"
)

// FileProcessor 负责管理文件解析和依赖提取的并发调度。
type FileProcessor struct {
	Language    model.Language
	OutputAST   bool
	FormatAST   bool
	Concurrency int
}

// NewFileProcessor 创建一个新的处理器实例。 接收一个包含所有配置的 ProcessorConfig 结构体。
func NewFileProcessor(lang model.Language, outputAST bool, formatAST bool, concurrency int) *FileProcessor {
	// 确保并发数有效，并使用 Concurrency 字段
	if concurrency <= 0 {
		concurrency = 4 // 默认并发数
	}
	return &FileProcessor{
		Language:    lang,
		OutputAST:   outputAST,
		FormatAST:   formatAST,
		Concurrency: concurrency,
	}
}

// ProcessFiles 实现了两阶段处理逻辑：收集定义 -> 提取关系。
func (fp *FileProcessor) ProcessFiles(ctx context.Context, filePaths []string) ([]*model.DependencyRelation, error) {
	if len(filePaths) == 0 {
		return nil, nil
	}

	// GlobalContext 是所有并发阶段共享和更新的中央符号表。
	globalContext := model.NewGlobalContext()

	// --- 阶段 1: 收集定义 (Definition Pass) ---
	// 使用 fp.Concurrency 访问嵌入的配置
	fmt.Printf("Phase 1: Collecting definitions from %d files with %d workers...\n", len(filePaths), fp.Concurrency)
	if _, err := fp.runPhase(ctx, filePaths, globalContext, fp.workerPhase1); err != nil {
		return nil, fmt.Errorf("phase 1 (definition collection) failed: %w", err)
	}

	// --- 阶段 2: 提取关系 (Relation Pass) ---
	// 使用 fp.Concurrency 访问嵌入的配置
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
type workerFunc func(context.Context, *sync.WaitGroup, <-chan string, chan interface{}, chan error, *model.GlobalContext)

// runPhase 调度并发工作协程并等待结果。
func (fp *FileProcessor) runPhase(ctx context.Context, filePaths []string, gc *model.GlobalContext, workerFn workerFunc) (interface{}, error) {
	filesChan := make(chan string, len(filePaths))
	// 结果通道用于收集 workerFn 返回的任意结果 (interface{})
	resultsChan := make(chan interface{}, len(filePaths))
	// 使用 fp.Concurrency
	errChan := make(chan error, fp.Concurrency)
	var wg sync.WaitGroup

	// 启动 worker 协程
	for i := 0; i < fp.Concurrency; i++ {
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
func (fp *FileProcessor) workerPhase1(ctx context.Context, wg *sync.WaitGroup, filesChan <-chan string, resultsChan chan interface{}, errChan chan error, gc *model.GlobalContext) {
	defer wg.Done()

	// 使用 fp.Language 访问嵌入的配置
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

	// 使用 fp.Language
	cot, extErr := collector.GetCollector(fp.Language)
	if extErr != nil {
		// 致命错误：无法获取 Collector
		select {
		case errChan <- fmt.Errorf("failed to get collector for %s: %w", fp.Language, extErr):
		default:
		}
		return
	}

	for filePath := range filesChan {
		// 调用 p.ParseFile 时传入 AST 输出配置
		rootNode, sourceBytes, err := p.ParseFile(filePath, fp.OutputAST, fp.FormatAST)
		if err != nil {
			fmt.Printf("[Warning P1] Skipping %s due to parsing error: %v\n", filePath, err)
			continue
		}

		// 调用 Collector 的定义收集方法
		fileContext, err := cot.CollectDefinitions(rootNode, filePath, sourceBytes)
		if err != nil {
			fmt.Printf("[Warning P1] Failed to collect definitions in %s: %v\n", filePath, err)
			continue
		}

		// 将结果安全地注册到全局上下文 (GlobalContext 是并发安全的)
		gc.RegisterFileContext(fileContext)
	}
}

// workerPhase2 负责执行 AST 解析和依赖关系提取。
func (fp *FileProcessor) workerPhase2(ctx context.Context, wg *sync.WaitGroup, filesChan <-chan string, resultsChan chan interface{}, errChan chan error, gCtx *model.GlobalContext) {
	defer wg.Done()

	// 使用 fp.Language
	p, pErr := parser.NewParser(fp.Language)
	if pErr != nil {
		select {
		case errChan <- fmt.Errorf("failed to create parser for %s: %w", fp.Language, pErr):
		default:
		}
		return
	}
	defer p.Close()

	// 使用 fp.Language
	ext, extErr := extractor.GetExtractor(fp.Language)
	if extErr != nil {
		select {
		case errChan <- fmt.Errorf("failed to get extractor for %s: %w", fp.Language, extErr):
		default:
		}
		return
	}

	for filePath := range filesChan {
		// Phase 2 不需要重新解析文件或输出 AST，直接从 GlobalContext 获取 AST 根节点
		if gCtx.FileContexts[filePath] == nil {
			fmt.Printf("[Warning P2] Skipping %s, file context not found.\n", filePath)
			continue
		}
		rootNode := gCtx.FileContexts[filePath].RootNode

		// 调用 Extractor 的关系提取方法，传入完整的 GlobalContext
		relations, err := ext.Extract(rootNode, filePath, gCtx)
		if err != nil {
			fmt.Printf("[Warning P2] Failed to extract relations in %s: %v\n", filePath, err)
			continue
		}

		// 将提取到的依赖关系发送给结果通道
		resultsChan <- relations
	}
}
