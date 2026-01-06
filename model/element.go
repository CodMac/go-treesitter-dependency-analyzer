package model

// --- 代码元素类型 (Code Element Kinds) ---

// ElementKind 是表示代码实体类型的字符串常量
type ElementKind string

const (
	File         ElementKind = "FILE"          // 基本结构体          -> 对应源文件
	Package      ElementKind = "PACKAGE"       // 基本结构体          -> 对应包        -> 适用 (Go, Java, C++)
	Module       ElementKind = "MODULE"        // 结构化代码块      -> 对应模块       -> 适用 (Python, Rust)
	Namespace    ElementKind = "NAMESPACE"     // 结构化代码块      -> 对应命名空间  -> 适用 (C++, C#)
	Class        ElementKind = "CLASS"         // 面向对象/复合类型    -> 对应类        -> 适用(Java, C++, Python, JS)
	Interface    ElementKind = "INTERFACE"     // 面向对象/复合类型    -> 对应接口       -> 适用(Java, Go, TS)
	Struct       ElementKind = "STRUCT"        // 面向对象/复合类型    -> 对应结构体   -> 适用(Go, C)
	Enum         ElementKind = "ENUM"          // 面向对象/复合类型    -> 对应枚举       -> 适用(Java, C++, Rust)
	EnumConstant ElementKind = "ENUM_CONSTANT" // 面向对象/复合类型    -> 对应枚举常量
	KAnnotation  ElementKind = "ANNOTATION"    // 面向对象/复合类型    -> 对应注解
	Trait        ElementKind = "TRAIT"         // 面向对象/复合类型    -> 对应特质       -> 适用接口 (Rust, Scala)
	Function     ElementKind = "FUNCTION"      // 可执行体       -> 对应独立函数  -> 适用(C, Go, JS)
	Method       ElementKind = "METHOD"        // 可执行体       -> 对应类/结构体的方法
	Macro        ElementKind = "MACRO"         // 可执行体       -> 对应预处理器宏     -> 适用(C/C++)
	Variable     ElementKind = "VARIABLE"      // 存储和声明          -> 对应局部/全局变量
	Constant     ElementKind = "CONSTANT"      // 存储和声明          -> 对应常量
	Field        ElementKind = "FIELD"         // 存储和声明          -> 对应类/结构体/枚举的成员或字段 -> 适用(Java, Go, C++)
	Type         ElementKind = "TYPE"          // 存储和声明          -> 对应自定义类型别名或基本类型引用
	Unknown      ElementKind = "UNKNOWN"       // 未知类型
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
	Kind          ElementKind `json:"Kind"`                // Kind: 元素的类型 (e.g., FUNCTION, CLASS, VARIABLE)
	Name          string      `json:"Name"`                // Name: 元素的短名称 (e.g., "main", "CalculateSum")
	QualifiedName string      `json:"QualifiedName"`       // QualifiedName: 元素的完整限定名称 (e.g., "pkg/util.Utility.CalculateSum")
	Path          string      `json:"Path"`                // Path: 元素所在的文件路径 (相对于项目根目录)
	Signature     string      `json:"Signature,omitempty"` // Signature: 元素的完整签名（针对函数/方法，包含参数和返回值类型）
	Location      *Location   `json:"Location,omitempty"`  // Location: 元素的位置
	Doc           string      `json:"Doc,omitempty"`       // Doc: 文档注释 (如 Javadoc, Go Doc)
	Comment       string      `json:"Comment,omitempty"`   // Comment: 普通注释 (行/块注释)
	Extra         *Extra      `json:"Extra,omitempty"`     // Extra 额外信息
}

// Extra CodeElement的额外信息。包含了跨语言（如Java和Go）通用的关键元数据。
type Extra struct {
	Modifiers   []string `json:"Modifiers,omitempty"`  // 修饰符列表 (e.g., "public", "private", "static", "final", "abstract")
	Annotations []string `json:"Annotation,omitempty"` // 注解列表 (e.g., "@Service")

	MethodExtra       *MethodExtra       `json:"MethodExtra,omitempty"` // 仅适用于 Method/Function
	ClassExtra        *ClassExtra        `json:"ClassExtra,omitempty"`  // 仅适用于 Class/Interface/Struct/Enum
	FieldExtra        *FieldExtra        `json:"FieldExtra,omitempty"`  // 仅适用于 Field/Constant
	EnumConstantExtra *EnumConstantExtra `json:"EnumConstantExtra,omitempty"`
}

// MethodExtra 存储方法或函数的特有信息
type MethodExtra struct {
	IsConstructor      bool     `json:"IsConstructor"`         // 是否是构造函数
	IncludeParamNameQN string   `json:"IncludeParamNameQN"`    // 包含参数名称的QN
	ReturnType         string   `json:"ReturnType,omitempty"`  // 适用于 Method/Function, Field (Java)
	ThrowsTypes        []string `json:"ThrowsTypes,omitempty"` // 抛出的异常类型 (Java) 或返回错误 (Go)
	Parameters         []string `json:"Parameters,omitempty"`  // 格式化的参数列表 (e.g., ["String name", "int count"])
}

// ClassExtra 存储类、接口、结构体或枚举的特有信息
type ClassExtra struct {
	SuperClass            string   `json:"SuperClass,omitempty"`            // 父类/继承的类 (Java/C++)
	ImplementedInterfaces []string `json:"ImplementedInterfaces,omitempty"` // 实现的接口 (Java) 或嵌入/组合的类型 (Go)
	IsAbstract            bool     `json:"IsAbstract,omitempty"`            // 是否是抽象的
	IsFinal               bool     `json:"IsFinal,omitempty"`               // 是否是 final
	IsBuiltin             bool     `json:"IsBuiltin,omitempty"`             // 是否是 库内置的  (e.g., ["String"])
}

// FieldExtra 存储字段或常量的特有信息
type FieldExtra struct {
	IsConstant bool   `json:"IsConstant,omitempty"` // 是否是常量 (final in Java, const in Go)
	IsParam    bool   `json:"IsParam,omitempty"`    // 是否是函数入参
	Type       string `json:"Type,omitempty"`       // 适用于 Field/Variable (Go/Java)
}

// EnumConstantExtra 保存实例化时传入的原始字符串参数列表
type EnumConstantExtra struct {
	Arguments []string `json:"arguments"` // 例如: USER_NOT_FOUND(404, "Not Found") -> ["404", "\"Not Found\""]
}
