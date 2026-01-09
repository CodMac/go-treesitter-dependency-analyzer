package core

import (
	"path/filepath"
	"sync"

	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type DefinitionEntry struct {
	Element  *model.CodeElement
	ParentQN string
	Node     *sitter.Node // 保留 AST 节点引用用于后期元数据填充
}

type ImportEntry struct {
	RawImportPath string            `json:"RawImportPath"`
	Alias         string            `json:"Alias"`
	Kind          model.ElementKind `json:"Kind"`
	IsWildcard    bool              `json:"IsWildcard"`
	Location      *model.Location   `json:"Location,omitempty"`
}

type FileContext struct {
	FilePath        string
	PackageName     string
	RootNode        *sitter.Node
	SourceBytes     *[]byte
	DefinitionsBySN map[string][]*DefinitionEntry
	Imports         map[string][]*ImportEntry
	mutex           sync.RWMutex
}

func NewFileContext(filePath string, rootNode *sitter.Node, sourceBytes *[]byte) *FileContext {
	return &FileContext{
		FilePath:        filePath,
		RootNode:        rootNode,
		SourceBytes:     sourceBytes,
		DefinitionsBySN: make(map[string][]*DefinitionEntry),
		Imports:         make(map[string][]*ImportEntry),
	}
}

func (fc *FileContext) AddDefinition(elem *model.CodeElement, parentQN string, node *sitter.Node) {
	fc.mutex.Lock()
	defer fc.mutex.Unlock()

	fc.DefinitionsBySN[elem.Name] = append(fc.DefinitionsBySN[elem.Name], &DefinitionEntry{Element: elem, ParentQN: parentQN, Node: node})
}

func (fc *FileContext) AddImport(alias string, imp *ImportEntry) {
	fc.mutex.Lock()
	defer fc.mutex.Unlock()
	fc.Imports[alias] = append(fc.Imports[alias], imp)
}

// --- 核心修改：GlobalContext 抽象化 ---

type GlobalContext struct {
	FileContexts    map[string]*FileContext
	DefinitionsByQN map[string][]*DefinitionEntry
	resolver        SymbolResolver // 持有具体语言的解析器
	mutex           sync.RWMutex
}

func NewGlobalContext(resolver SymbolResolver) *GlobalContext {
	return &GlobalContext{
		FileContexts:    make(map[string]*FileContext),
		DefinitionsByQN: make(map[string][]*DefinitionEntry),
		resolver:        resolver,
	}
}

// RegisterFileContext 逻辑现在调用 resolver 处理包名
func (gc *GlobalContext) RegisterFileContext(fc *FileContext) {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	gc.FileContexts[fc.FilePath] = fc

	// 1. 注册文件节点
	fileElem := &model.CodeElement{
		Kind:          model.File,
		Name:          filepath.Base(fc.FilePath),
		QualifiedName: fc.FilePath,
		Path:          fc.FilePath,
	}
	gc.DefinitionsByQN[fc.FilePath] = []*DefinitionEntry{{Element: fileElem}}

	// 2. 委托 Resolver 处理包/命名空间注册 (Java 拆分, Go 不拆)
	gc.resolver.RegisterPackage(gc, fc.PackageName)

	// 3. 注册文件内定义
	for _, entries := range fc.DefinitionsBySN {
		for _, entry := range entries {
			gc.DefinitionsByQN[entry.Element.QualifiedName] = append(gc.DefinitionsByQN[entry.Element.QualifiedName], entry)
		}
	}
}

// ResolveSymbol 彻底由 Resolver 驱动
func (gc *GlobalContext) ResolveSymbol(fc *FileContext, symbol string) []*DefinitionEntry {
	return gc.resolver.Resolve(gc, fc, symbol)
}

func (gc *GlobalContext) BuildQualifiedName(parentQN, name string) string {
	return gc.resolver.BuildQualifiedName(parentQN, name)
}

func (gc *GlobalContext) RLock() { gc.mutex.RLock() }

func (gc *GlobalContext) RUnlock() { gc.mutex.RUnlock() }
