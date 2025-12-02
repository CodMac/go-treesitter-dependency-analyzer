package extractor

import (
	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	"sync"
)

// GlobalContext 存储了项目中所有文件的定义信息，用于跨文件 QN 解析。
type GlobalContext struct {
	FileContexts      map[string]*FileContext   // FilePath -> *FileContext
	GlobalDefinitions map[string][]*SymbolEntry // QN -> []SymbolEntry: 存储所有定义元素的全局映射。 (由于可能存在跨语言或多重定义，使用切片存储)
	mu                sync.RWMutex
}

// FileContext 存储了单个文件的定义信息
type FileContext struct {
	FilePath    string
	PackageName string
	Definitions map[string]*SymbolEntry // QN -> *SymbolEntry: 存储文件内所有定义的元素
}

// SymbolEntry 代表符号表中的一个实体定义
type SymbolEntry struct {
	Element  *model.CodeElement
	ParentQN string
}

// NewGlobalContext 初始化全局上下文
func NewGlobalContext() *GlobalContext {
	return &GlobalContext{
		FileContexts:      make(map[string]*FileContext),
		GlobalDefinitions: make(map[string][]*SymbolEntry),
	}
}

// NewFileContext 初始化文件上下文
func NewFileContext(filePath string) *FileContext {
	return &FileContext{
		FilePath:    filePath,
		Definitions: make(map[string]*SymbolEntry),
	}
}

// RegisterFileContext 将单个文件的上下文添加到全局上下文，并更新全局定义。
func (gc *GlobalContext) RegisterFileContext(fc *FileContext) {
	gc.mu.Lock()
	defer gc.mu.Unlock()

	gc.FileContexts[fc.FilePath] = fc

	// 合并定义到全局符号表
	for qn, entry := range fc.Definitions {
		gc.GlobalDefinitions[qn] = append(gc.GlobalDefinitions[qn], entry)
	}
}

// ResolveQN 在全局上下文中查找 QN。
func (gc *GlobalContext) ResolveQN(qn string) []*SymbolEntry {
	gc.mu.RLock()
	defer gc.mu.RUnlock()

	return gc.GlobalDefinitions[qn]
}
