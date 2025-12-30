package processor

import (
	"context"
	"path/filepath"
	"sync"

	"github.com/CodMac/go-treesitter-dependency-analyzer/collector"
	"github.com/CodMac/go-treesitter-dependency-analyzer/extractor"
	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	"github.com/CodMac/go-treesitter-dependency-analyzer/parser"
)

type FileProcessor struct {
	Language    model.Language
	OutputAST   bool
	FormatAST   bool
	Concurrency int
}

func NewFileProcessor(lang model.Language, outputAST, formatAST bool, concurrency int) *FileProcessor {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &FileProcessor{
		Language:    lang,
		OutputAST:   outputAST,
		FormatAST:   formatAST,
		Concurrency: concurrency,
	}
}

func (fp *FileProcessor) ProcessFiles(ctx context.Context, rootPath string, filePaths []string) ([]*model.DependencyRelation, *model.GlobalContext, error) {
	globalContext := model.NewGlobalContext()
	absRoot, _ := filepath.Abs(rootPath) // 预先计算绝对根路径

	// --- 阶段 1: 收集定义 ---
	err := fp.runParallel(filePaths, func(path string, p parser.Parser) error {
		root, source, err := p.ParseFile(path, fp.OutputAST, fp.FormatAST)
		if err != nil {
			return err
		}

		cot, _ := collector.GetCollector(fp.Language)
		fCtx, err := cot.CollectDefinitions(root, path, source)
		if err != nil {
			return err
		}

		// 在注册前进行归一化
		for _, entries := range fCtx.DefinitionsBySN {
			for _, entry := range entries {
				globalContext.NormalizeElementPaths(entry.Element, absRoot)
			}
		}

		globalContext.RegisterFileContext(fCtx)
		return nil
	})

	// --- 阶段 2: 提取关系 ---
	var allRelations []*model.DependencyRelation
	var mu sync.Mutex
	err = fp.runParallel(filePaths, func(path string, p parser.Parser) error {
		ext, _ := extractor.GetExtractor(fp.Language)
		rels, _ := ext.Extract(path, globalContext)

		mu.Lock()
		for _, rel := range rels {
			// 处理关系中的 Location 路径
			if rel.Location != nil && filepath.IsAbs(rel.Location.FilePath) {
				if relPath, err := filepath.Rel(absRoot, rel.Location.FilePath); err == nil {
					rel.Location.FilePath = relPath
				}
			}
			allRelations = append(allRelations, rel)
		}
		mu.Unlock()
		return nil
	})

	return allRelations, globalContext, err
}

// runParallel 抽象了并发调度逻辑，自动管理线程池和 Parser 的生命周期
func (fp *FileProcessor) runParallel(paths []string, task func(string, parser.Parser) error) error {
	pathChan := make(chan string, len(paths))
	for _, p := range paths {
		pathChan <- p
	}
	close(pathChan)

	var wg sync.WaitGroup
	var firstErr error
	var errOnce sync.Once

	for i := 0; i < fp.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// 每个协程创建一个独立的解析器，避免线程竞争
			p, err := parser.NewParser(fp.Language)
			if err != nil {
				errOnce.Do(func() { firstErr = err })
				return
			}
			defer p.Close()

			for path := range pathChan {
				if err := task(path, p); err != nil {
					errOnce.Do(func() { firstErr = err })
					return // 遇到错误则停止当前 Worker
				}
			}
		}()
	}
	wg.Wait()
	return firstErr
}
