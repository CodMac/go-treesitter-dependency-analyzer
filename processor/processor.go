package processor

import (
	"context"
	"path/filepath"
	"strings"
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
	kipBuiltin  bool // 是否跳过内置类/标准库的依赖提取
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
	absRoot, _ := filepath.Abs(rootPath)

	// --- 阶段 1: 收集定义 (Parallel) ---
	// 目的：把所有类、方法、文件、包节点先塞进 GlobalContext
	err := fp.runParallel(filePaths, func(path string, p parser.Parser) error {
		root, source, err := p.ParseFile(path, fp.OutputAST, fp.FormatAST)
		if err != nil {
			return err
		}

		cot, err := collector.GetCollector(fp.Language)
		if err != nil {
			return err
		}

		// 归一化路径
		relPath, err := filepath.Rel(absRoot, path)
		if err != nil {
			return err
		}

		fCtx, err := cot.CollectDefinitions(root, relPath, source)
		if err != nil {
			return err
		}

		globalContext.RegisterFileContext(fCtx)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	// --- 阶段 2: 补充层级包含关系 (Sequential) ---
	// 目的：在分析依赖前，先拉起整棵“组织架构树”
	hierarchyRels := fp.complementHierarchy(globalContext)

	// --- 阶段 3: 提取逻辑依赖关系 (Parallel) ---
	// 目的：在完整的树状背景下，勾勒类与类之间的调用线条
	var allRelations []*model.DependencyRelation
	allRelations = append(allRelations, hierarchyRels...)

	var mu sync.Mutex
	err = fp.runParallel(filePaths, func(path string, p parser.Parser) error {
		ext, err := extractor.GetExtractor(fp.Language)
		if err != nil {
			return err
		}

		// 归一化路径
		relPath, err := filepath.Rel(absRoot, path)
		if err != nil {
			return err
		}

		rels, err := ext.Extract(relPath, globalContext)
		if err != nil {
			return err
		}

		mu.Lock()
		defer mu.Unlock()
		for _, rel := range rels {
			// 归一化关系中的位置信息
			if rel.Location != nil && filepath.IsAbs(rel.Location.FilePath) {
				if relPath, err := filepath.Rel(absRoot, rel.Location.FilePath); err == nil {
					rel.Location.FilePath = relPath
				}
			}
			allRelations = append(allRelations, rel)
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	return allRelations, globalContext, nil
}

// complementHierarchy 负责构建 Package -> SubPackage -> File -> Class 的树状包含关系
func (fp *FileProcessor) complementHierarchy(gc *model.GlobalContext) []*model.DependencyRelation {
	hierarchyRels := make(map[string]*model.DependencyRelation)

	gc.RLock()
	defer gc.RUnlock()

	for _, fCtx := range gc.FileContexts {
		// 1. Package -> File
		if fCtx.PackageName != "" {
			pkgFileKey := "pf:" + fCtx.PackageName + ">" + fCtx.FilePath
			hierarchyRels[pkgFileKey] = &model.DependencyRelation{
				Type:   "CONTAINS",
				Source: &model.CodeElement{Kind: model.Package, QualifiedName: fCtx.PackageName},
				Target: &model.CodeElement{Kind: model.File, QualifiedName: fCtx.FilePath},
			}

			// 2. Package -> SubPackage (递归向上推导)
			parts := strings.Split(fCtx.PackageName, ".")
			for i := len(parts) - 1; i > 0; i-- {
				parentPkg := strings.Join(parts[:i], ".")
				subPkg := strings.Join(parts[:i+1], ".")

				pkgPkgKey := "pp:" + parentPkg + ">" + subPkg
				if _, exists := hierarchyRels[pkgPkgKey]; exists {
					break // 向上层级已处理过，直接跳出
				}
				hierarchyRels[pkgPkgKey] = &model.DependencyRelation{
					Type:   "CONTAINS",
					Source: &model.CodeElement{Kind: model.Package, QualifiedName: parentPkg},
					Target: &model.CodeElement{Kind: model.Package, QualifiedName: subPkg},
				}
			}
		}

		// 3. File -> TopLevelElement (Class/Interface etc.)
		for _, entries := range fCtx.DefinitionsBySN {
			for _, entry := range entries {
				// 准则：只有没有 ParentQN 或者 ParentQN 等于当前包名的元素才挂在文件下
				// 避免方法节点也被平铺到文件节点中
				if entry.ParentQN == "" || entry.ParentQN == fCtx.PackageName {
					fileElemKey := "fe:" + fCtx.FilePath + ">" + entry.Element.QualifiedName
					hierarchyRels[fileElemKey] = &model.DependencyRelation{
						Type:   "CONTAINS",
						Source: &model.CodeElement{Kind: model.File, QualifiedName: fCtx.FilePath},
						Target: entry.Element,
					}
				}
			}
		}
	}

	// 转换为切片返回
	result := make([]*model.DependencyRelation, 0, len(hierarchyRels))
	for _, rel := range hierarchyRels {
		result = append(result, rel)
	}
	return result
}

// runParallel 并发调度器
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
			p, err := parser.NewParser(fp.Language)
			if err != nil {
				errOnce.Do(func() { firstErr = err })
				return
			}
			defer p.Close()

			for path := range pathChan {
				if err := task(path, p); err != nil {
					errOnce.Do(func() { firstErr = err })
					return
				}
			}
		}()
	}
	wg.Wait()
	return firstErr
}
