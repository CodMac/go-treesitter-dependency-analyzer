package model

// --- 代码元素类型 (Code Element Kinds) ---

// ElementKind 是表示代码实体类型的字符串常量
type ElementKind string

const (
	// 基本结构体
	File    ElementKind = "FILE"    // 对应源文件
	Package ElementKind = "PACKAGE" // 对应包、模块、命名空间 (Go, Java, C++)

	// 结构化代码块
	Module    ElementKind = "MODULE"    // 对应模块 (Python, Rust)
	Namespace ElementKind = "NAMESPACE" // 对应命名空间 (C++, C#)

	// 面向对象/复合类型
	Class       ElementKind = "CLASS"      // 对应类 (Java, C++, Python, JS)
	Interface   ElementKind = "INTERFACE"  // 对应接口 (Java, Go, TS)
	Struct      ElementKind = "STRUCT"     // 对应结构体 (Go, C)
	Enum        ElementKind = "ENUM"       // 对应枚举 (Java, C++, Rust)
	Trait       ElementKind = "TRAIT"      // 对应特质/接口 (Rust, Scala)
	Annotationn ElementKind = "ANNOTATION" //  对应注解 (Java, Python)

	// 可执行体
	Function ElementKind = "FUNCTION" // 对应独立函数 (C, Go, JS)
	Method   ElementKind = "METHOD"   // 对应类/结构体的方法
	Macro    ElementKind = "MACRO"    // 对应预处理器宏 (C/C++)

	// 存储和声明
	Variable ElementKind = "VARIABLE" // 对应局部/全局变量
	Constant ElementKind = "CONSTANT" // 对应常量
	Field    ElementKind = "FIELD"    // 对应类/结构体/枚举的成员或字段 (Java, Go, C++)
	Type     ElementKind = "TYPE"     // 对应自定义类型别名或基本类型引用

	// 未知类型
	Unknown ElementKind = "UNKNOWN"
)

// Location 描述了代码元素或依赖关系在源码中的位置
type Location struct {
	FilePath    string `json:"FilePath"`
	StartLine   int    `json:"StartLine"`
	EndLine     int    `json:"EndLine"`
	StartColumn int    `json:"StartColumn"`
	EndColumn   int    `json:"EndColumn"`
}

// CodeElement 描述了源码中的一个可识别实体（Source 或 Target）
type CodeElement struct {
	Kind          ElementKind `json:"Kind"`                    // Kind: 元素的类型 (e.g., FUNCTION, CLASS, VARIABLE)
	Name          string      `json:"Name"`                    // Name: 元素的短名称 (e.g., "main", "CalculateSum")
	QualifiedName string      `json:"QualifiedName"`           // QualifiedName: 元素的完整限定名称 (e.g., "pkg/util.Utility.CalculateSum")
	Path          string      `json:"Path"`                    // Path: 元素所在的文件路径 (相对于项目根目录)
	Signature     string      `json:"Signature,omitempty"`     // Signature: 元素的完整签名（针对函数/方法，包含参数和返回值类型）
	StartLocation *Location   `json:"StartLocation,omitempty"` // StartLocation: 元素的起始位置
	EndLocation   *Location   `json:"EndLocation,omitempty"`   // EndLocation: 元素的结束位置
}
