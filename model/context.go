package model

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// DefinitionEntry 存储了一个符号定义的完整信息，以及它所属的父元素QN。
type DefinitionEntry struct {
	Element  *CodeElement // 符号本身
	ParentQN string       // 父元素的QualifiedName
}

// ImportEntry 描述一个导入声明的详细信息
type ImportEntry struct {
	RawImportPath string      `json:"RawImportPath"` // 原始导入路径 (如 "java.util.List")
	Alias         string      `json:"Alias"`         // 别名或短名称 (如 "List")
	Kind          ElementKind `json:"Kind"`          // 导入的类型：Class, Package, Constant(static import)等
	IsWildcard    bool        `json:"IsWildcard"`    // 是否为通配符导入 (.* 或 /...)
	Location      *Location   `json:"Location,omitempty"`
}

// FileContext 存储了单个文件的所有符号定义、包名和源代码内容。
type FileContext struct {
	FilePath        string                        // 文件路径
	PackageName     string                        // 文件所属的包/模块名 (例如 Java 的 package)
	RootNode        *sitter.Node                  // AST根节点
	SourceBytes     *[]byte                       // 源码内容 (指针)
	DefinitionsBySN map[string][]*DefinitionEntry // 局部定义查找 (短名称 -> 定义列表), 使用切片支持重载（如多个构造函数）或内部类与方法同名的情况, Key为短名称（不包含括号、泛型、参数...）
	Imports         map[string]*ImportEntry       // Key 为 Alias (如 "List" 或 "L")
	mutex           sync.RWMutex
}

// NewFileContext 创建一个新的 FileContext 实例。
func NewFileContext(filePath string, rootNode *sitter.Node, sourceBytes *[]byte) *FileContext {
	return &FileContext{
		FilePath:        filePath,
		RootNode:        rootNode,
		SourceBytes:     sourceBytes,
		DefinitionsBySN: make(map[string][]*DefinitionEntry),
		Imports:         make(map[string]*ImportEntry),
	}
}

// AddDefinition 将一个符号定义添加到 FileContext 中。
func (fc *FileContext) AddDefinition(elem *CodeElement, parentQN string) {
	fc.mutex.Lock()
	defer fc.mutex.Unlock()

	entry := &DefinitionEntry{
		Element:  elem,
		ParentQN: parentQN,
	}

	fc.DefinitionsBySN[elem.Name] = append(fc.DefinitionsBySN[elem.Name], entry)
}

// AddImport 辅助方法：方便 Collector 调用
func (fc *FileContext) AddImport(alias string, imp *ImportEntry) {
	fc.mutex.Lock()
	defer fc.mutex.Unlock()
	fc.Imports[alias] = imp
}

// GlobalContext 存储了整个项目范围内的符号信息。
type GlobalContext struct {
	FileContexts    map[string]*FileContext       // 文件路径 -> FileContext
	DefinitionsByQN map[string][]*DefinitionEntry // 全局定义索引 (QN -> 定义列表), 支持同名 QN（处理多版本库或增量扫描时的冲突）
	mutex           sync.RWMutex
}

// NewGlobalContext 创建一个新的 GlobalContext 实例。
func NewGlobalContext() *GlobalContext {
	return &GlobalContext{
		FileContexts:    make(map[string]*FileContext),
		DefinitionsByQN: make(map[string][]*DefinitionEntry),
	}
}

// RegisterFileContext 将 FileContext 的信息同步到全局索引。
func (gc *GlobalContext) RegisterFileContext(fc *FileContext) {
	gc.mutex.Lock()
	defer gc.mutex.Unlock()

	gc.FileContexts[fc.FilePath] = fc

	// 1. 自动生成并注册 FILE 节点
	fileElem := &CodeElement{
		Kind:          File,
		Name:          filepath.Base(fc.FilePath),
		QualifiedName: fc.FilePath, // 此时路径已归一化
		Path:          fc.FilePath,
	}
	gc.DefinitionsByQN[fc.FilePath] = append(gc.DefinitionsByQN[fc.FilePath], &DefinitionEntry{
		Element: fileElem,
	})

	// 2. 注册文件内的其他定义
	for _, entries := range fc.DefinitionsBySN {
		for _, entry := range entries {
			qn := entry.Element.QualifiedName
			gc.DefinitionsByQN[qn] = append(gc.DefinitionsByQN[qn], entry)
		}
	}
}

// ResolveSymbol 尝试解析一个标识符的具体定义。
// 优先级：局部定义 > 精确导入 > 同包定义 > 通配符导入 > 绝对路径查找
func (gc *GlobalContext) ResolveSymbol(fc *FileContext, symbol string) []*DefinitionEntry {
	gc.mutex.RLock()
	defer gc.mutex.RUnlock()

	// 1. 局部定义优先 (当前文件内的类、方法、内部类等)
	if defs, ok := fc.DefinitionsBySN[symbol]; ok {
		return defs
	}

	// 2. 检查精确 Import (例如 "List" -> ImportEntry{RawImportPath: "java.util.List"})
	if imp, ok := fc.Imports[symbol]; ok {
		if defs, found := gc.DefinitionsByQN[imp.RawImportPath]; found {
			return defs
		}
	}

	// 3. 尝试当前包前缀 (隐式引用同包下的其他类)
	if fc.PackageName != "" {
		pkgQN := BuildQualifiedName(fc.PackageName, symbol)
		if defs, ok := gc.DefinitionsByQN[pkgQN]; ok {
			return defs
		}
	}

	// 4. 处理通配符导入 (例如 Java 中的 import java.util.*)
	// 遍历所有标记为 IsWildcard 的导入，尝试拼接并查找
	for _, imp := range fc.Imports {
		if imp.IsWildcard {
			// 去掉路径末尾的 "*" 并拼接当前符号
			// 例如 "java.util.*" -> "java.util." + "List"
			basePath := strings.TrimSuffix(imp.RawImportPath, "*")
			wildcardQN := basePath + symbol
			if defs, ok := gc.DefinitionsByQN[wildcardQN]; ok {
				return defs
			}
		}
	}

	// 5. 兜底：直接按 QN 查找 (处理代码中使用全限定名调用的情况)
	if defs, ok := gc.DefinitionsByQN[symbol]; ok {
		return defs
	}

	return nil
}

// NormalizeElementPaths 将元素中的绝对路径转换为相对于 rootPath 的相对路径
func (gc *GlobalContext) NormalizeElementPaths(elem *CodeElement, rootPath string) {
	if elem == nil || rootPath == "" {
		return
	}

	// 1. 处理 Path 字段
	if elem.Path != "" && filepath.IsAbs(elem.Path) {
		if rel, err := filepath.Rel(rootPath, elem.Path); err == nil {
			elem.Path = rel
		}
	}

	// 2. 处理 FILE 类型的 QualifiedName (通常是全路径)
	if elem.Kind == File && filepath.IsAbs(elem.QualifiedName) {
		if rel, err := filepath.Rel(rootPath, elem.QualifiedName); err == nil {
			elem.QualifiedName = rel
		}
	}
}

// BuildQualifiedName 构建限定名称 (Qualified Name, QN)
func BuildQualifiedName(parentQN, name string) string {
	if parentQN == "" || parentQN == "." {
		return name
	}
	return fmt.Sprintf("%s.%s", parentQN, name)
}
