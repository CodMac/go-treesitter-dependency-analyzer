package processor

import (
	"context"
	"fmt"
	"sync"

	"go-treesitter-dependency-analyzer/extractor"
	"go-treesitter-dependency-analyzer/model"
	"go-treesitter-dependency-analyzer/parser"
)

// FileProcessor 负责并发处理文件列表，并聚合所有提取的依赖关系。
type FileProcessor struct {
	Language model.Language
	Workers  int // 并发协程数量
}

// NewFileProcessor 创建 FileProcessor 实例
func NewFileProcessor(lang model.Language, workers int) *FileProcessor {
	if workers <= 0 {
		workers = 4 // 默认并发数
	}
	return &FileProcessor{
		Language: lang,
		Workers:  workers,
	}
}

// ProcessFiles 实现了两阶段处理逻辑。
func (fp *FileProcessor) ProcessFiles(ctx context.Context, filePaths []string) ([]*model.DependencyRelation, error) {
	if len(filePaths) == 0 {
		return nil, nil
	}
	
	globalContext := model.NewGlobalContext()
	
	// --- 阶段 1: 收集定义 (Collect Definitions) ---
	fmt.Printf("Phase 1: Collecting definitions from %d files...\n", len(filePaths))
	if err := fp.runPhase(ctx, filePaths, globalContext, fp.workerPhase1); err != nil {
		return nil, fmt.Errorf("phase 1 (definition collection) failed: %w", err)
	}

	// --- 阶段 2: 提取关系 (Extract Relations) ---
	fmt.Printf("Phase 2: Extracting dependencies...\n")
	relations, err := fp.runPhase(ctx, filePaths, globalContext, fp.workerPhase2)
	if err != nil {
		return nil, fmt.Errorf("phase 2 (relation extraction) failed: %w", err)
	}
	
	return relations.([]*model.DependencyRelation), nil
}

// workerFunc 定义了阶段工作函数的签名
type workerFunc func(context.Context, *sync.WaitGroup, <-chan string, chan interface{}, chan error, *model.GlobalContext)

// runPhase 运行一个并发阶段
func (fp *FileProcessor) runPhase(ctx context.Context, filePaths []string, gc *model.GlobalContext, workerFn workerFunc) (interface{}, error) {
	filesChan := make(chan string, len(filePaths))
	// resultsChan 在 Phase 1 传回 *model.FileContext，在 Phase 2 传回 []*model.DependencyRelation
	resultsChan := make(chan interface{}, len(filePaths)) 
	errChan := make(chan error, fp.Workers)
	var wg sync.WaitGroup

	for i := 0; i < fp.Workers; i++ {
		wg.Add(1)
		go workerFn(ctx, &wg, filesChan, resultsChan, errChan, gc)
	}

	for _, path := range filePaths {
		filesChan <- path
	}
	close(filesChan)

	go func() {
		wg.Wait()
		close(resultsChan)
		close(errChan)
	}()

	var allResults []interface{}
	for res := range resultsChan {
		allResults = append(allResults, res)
	}

	if err := <-errChan; err != nil {
		return nil, err
	}
	
	// Phase 1 成功返回 nil， Phase 2 返回 relations
	if workerFn == fp.workerPhase1 {
		return nil, nil
	}
	
	// 聚合 Phase 2 的依赖关系
	var allRelations []*model.DependencyRelation
	for _, res := range allResults {
		if rels, ok := res.([]*model.DependencyRelation); ok {
			allRelations = append(allRelations, rels...)
		}
	}
	return allRelations, nil
}

// workerPhase1 负责执行第一阶段：文件解析和定义收集。
func (fp *FileProcessor) workerPhase1(ctx context.Context, wg *sync.WaitGroup, filesChan <-chan string, resultsChan chan interface{}, errChan chan error, gc *model.GlobalContext) {
	defer wg.Done()
	
	// 确保每个 worker 都有自己的 parser 和 extractor
	// ... (初始化 parser 和 extractor 的代码保持不变) ...

	for filePath := range filesChan {
		rootNode, err := p.ParseFile(filePath)
		if err != nil {
			fmt.Printf("[Warning P1] Skipping %s: %v\n", filePath, err)
			continue
		}
		
		// 所有的 Extractor 必须实现 CollectDefinitions 方法
		collector, ok := ext.(extractor.DefinitionCollector) 
		if !ok {
			// 如果 Extractor 没有实现 DefinitionCollector 接口，这是一个错误
			errChan <- fmt.Errorf("extractor for %s does not implement DefinitionCollector", fp.Language)
			return
		}
		
		// 收集定义
		fileContext, err := collector.CollectDefinitions(rootNode, filePath)
		if err != nil {
			fmt.Printf("[Warning P1] Failed to collect definitions in %s: %v\n", filePath, err)
			continue
		}
		
		// 注册到全局上下文 (带锁)
		gc.RegisterFileContext(fileContext)
	}
}

// workerPhase2 负责执行第二阶段：依赖关系提取。
func (fp *FileProcessor) workerPhase2(ctx context.Context, wg *sync.WaitGroup, filesChan <-chan string, resultsChan chan interface{}, errChan chan error, gc *model.GlobalContext) {
	defer wg.Done()

	// ... (初始化 parser 和 extractor 的代码保持不变) ...

	for filePath := range filesChan {
		rootNode, err := p.ParseFile(filePath)
		if err != nil {
			// 在 Phase 1 已经警告过解析错误，这里跳过
			continue
		}
		
		// 确保 Extractor 实现了 ContextExtractor 接口
		contextExt, ok := ext.(extractor.ContextExtractor) 
		if !ok {
			errChan <- fmt.Errorf("extractor for %s does not implement ContextExtractor", fp.Language)
			return
		}
		
		// 提取关系，并传入全局上下文
		relations, err := contextExt.Extract(rootNode, filePath, gc)
		if err != nil {
			fmt.Printf("[Warning P2] Failed to extract relations in %s: %v\n", filePath, err)
			continue
		}
		
		resultsChan <- relations
	}
}