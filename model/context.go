package model

import (
	"fmt"
	"sync"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// DefinitionEntry 存储了一个符号定义的完整信息，以及它所属的父元素QN。
type DefinitionEntry struct {
	Element  *CodeElement // 符号本身
	ParentQN string       // 父元素的QualifiedName
}

// FileContext 存储了单个文件的所有符号定义、包名和源代码内容。
type FileContext struct {
	FilePath    string                      // 文件路径
	PackageName string                      // 文件所属的包/模块名
	RootNode    *sitter.Node                // AST根节点
	SourceBytes *[]byte                     // 源码内容 (指针，避免大内存复制)
	Definitions map[string]*DefinitionEntry // 短名称 -> 定义实体 (例如: "User" -> Class: "com.pkg.User")
	mutex       sync.RWMutex                // 保护 Definitions
}

// NewFileContext 创建一个新的 FileContext 实例。
func NewFileContext(filePath string, rootNode *sitter.Node, sourceBytes *[]byte) *FileContext {
	return &FileContext{
		FilePath:    filePath,
		RootNode:    rootNode,
		SourceBytes: sourceBytes,
		Definitions: make(map[string]*DefinitionEntry),
	}
}

// AddDefinition 将一个符号定义安全地添加到 FileContext 中。
// Key 使用元素的短名称 (elem.Name)。
func (fc *FileContext) AddDefinition(elem *CodeElement, parentQN string) {
	fc.mutex.Lock()
	defer fc.mutex.Unlock()
	fc.Definitions[elem.Name] = &DefinitionEntry{
		Element:  elem,
		ParentQN: parentQN,
	}
}

// GlobalContext 存储了整个项目范围内的符号信息。
// 它是跨文件、跨阶段共享和更新的中央数据结构。
type GlobalContext struct {
	// FileContexts: 文件路径 -> FileContext (存储文件级信息和定义)
	FileContexts map[string]*FileContext
	// DefinitionsByQN: 限定名称 -> DefinitionEntry (用于快速全局查找)
	DefinitionsByQN map[string]*DefinitionEntry
	mutex           sync.RWMutex
}

// NewGlobalContext 创建一个新的 GlobalContext 实例。
func NewGlobalContext() *GlobalContext {
	return &GlobalContext{
		FileContexts:    make(map[string]*FileContext),
		DefinitionsByQN: make(map[string]*DefinitionEntry),
	}
}

// RegisterFileContext 将 FileContext 注册到 GlobalContext 中。
// 这个方法是并发安全的。
func (gc *GlobalContext) RegisterFileContext(fc *FileContext) {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	gc.FileContexts[fc.FilePath] = fc

	// 将所有符号定义注册到全局 QN 映射
	for _, entry := range fc.Definitions {
		gc.DefinitionsByQN[entry.Element.QualifiedName] = entry
	}
}

// ResolveQN 通过限定名称 (QN) 查找符号定义。
// 如果找不到精确匹配，则返回空列表。
func (gc *GlobalContext) ResolveQN(qualifiedName string) []*DefinitionEntry {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	// 优先精确匹配
	if entry, ok := gc.DefinitionsByQN[qualifiedName]; ok {
		return []*DefinitionEntry{entry}
	}

	return nil
}

// BuildQualifiedName 构建限定名称 (Qualified Name, QN)
func BuildQualifiedName(parentQN, name string) string {
	if parentQN == "" || parentQN == "." {
		return name
	}
	return fmt.Sprintf("%s.%s", parentQN, name)
}
