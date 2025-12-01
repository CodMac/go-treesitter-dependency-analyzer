package java

import (
	"go-treesitter-dependency-analyzer/model"
)

// SymbolEntry 代表符号表中的一个实体定义
type SymbolEntry struct {
	Element *model.CodeElement
	ParentQN string // 父级元素的限定名 (e.g., 所属的 Class QN)
}

// FileContext 存储了单个文件的所有定义信息，作为 QN 解析的上下文
type FileContext struct {
	FilePath string
	PackageName string // 当前文件的包名
	
	// QN -> SymbolEntry: 存储所有定义的元素 (Class, Method, Field, etc.)
	Definitions map[string]*SymbolEntry 
	
	// Name -> QN: 存储简短名称到限定名 (QN) 的映射。
	// 注意：由于存在重名，这里需要更复杂的逻辑，但简化为只存储最顶层/最内层的定义。
	// 例如: "MyClass" -> "com.example.MyClass"
	ShortNameMap map[string]string 
}

// NewFileContext 初始化上下文
func NewFileContext(filePath string) *FileContext {
	return &FileContext{
		FilePath: filePath,
		Definitions: make(map[string]*SymbolEntry),
		ShortNameMap: make(map[string]string),
	}
}

// AddDefinition 记录一个符号定义
func (c *FileContext) AddDefinition(elem *model.CodeElement, parentQN string) {
	if elem.QualifiedName == "" {
		// 忽略没有 QN 的元素
		return
	}
	c.Definitions[elem.QualifiedName] = &SymbolEntry{
		Element: elem,
		ParentQN: parentQN,
	}
	
	// 记录短名称映射
	if elem.Name != "" {
		c.ShortNameMap[elem.Name] = elem.QualifiedName
	}
}

// buildQualifiedName 根据父 QN 和当前元素的名称构建新的 QN。
func (c *FileContext) buildQualifiedName(parentQN, name string) string {
	if parentQN == "" || c.PackageName == name {
		// 如果没有父级 QN 或 QN 已经是包名，则直接使用名称。
		return name
	}
	return fmt.Sprintf("%s.%s", parentQN, name)
}