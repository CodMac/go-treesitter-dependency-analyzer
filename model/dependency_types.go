package model

// --- 依赖关系类型 (Dependency Relation Types) ---

// DependencyType 是表示依赖关系的字符串常量
type DependencyType string

const (
	// Call: Source calls Target function/method.
	Call DependencyType = "CALL" 
	// Import: Source file/module imports Target package/module.
	Import DependencyType = "IMPORT"
	// Contain: Source element contains Target element (e.g., Class contains Member).
	Contain DependencyType = "CONTAIN"
	// Parameter: Function/Method uses Target type as a parameter.
	Parameter DependencyType = "PARAMETER"
	// Return: Function/Method returns Target type.
	Return DependencyType = "RETURN"
	// Throw: Function/Method throws Target exception type.
	Throw DependencyType = "THROW"
	// Implement: Class implements Interface, or function implements Prototype.
	Implement DependencyType = "IMPLEMENT"
	// Extend: Class inherits from another Base Class.
	Extend DependencyType = "EXTEND"
	// Create: Function/Method instantiates/creates an object of Target type.
	Create DependencyType = "CREATE"
	// Use: Source expression/block uses Target variable/constant/type (General Reference).
	Use DependencyType = "USE"
	// Cast: Expression is explicitly cast to Target type.
	Cast DependencyType = "CAST"
	// ImplLink: Implementation Link (e.g., C/C++ linking prototype to implementation).
	ImplLink DependencyType = "IMPL_LINK"
	// Annotation: Code element is decorated by Target Annotation/Decorator.
	Annotation DependencyType = "ANNOTATION"
	// Mixin: Class includes/mixes in Target Module (e.g., Ruby).
	Mixin DependencyType = "MIXIN"
)

// --- 代码元素类型 (Code Element Kinds) ---

// ElementKind 是表示代码实体类型的字符串常量
type ElementKind string

const (
	// 基本结构体
	File     ElementKind = "FILE"     // 对应源文件
	Package  ElementKind = "PACKAGE"  // 对应包、模块、命名空间 (Go, Java, C++)
	
	// 结构化代码块
	Module   ElementKind = "MODULE"   // 对应模块 (Python, Rust)
	Namespace ElementKind = "NAMESPACE" // 对应命名空间 (C++, C#)
	
	// 面向对象/复合类型
	Class    ElementKind = "CLASS"    // 对应类 (Java, C++, Python, JS)
	Interface ElementKind = "INTERFACE" // 对应接口 (Java, Go, TS)
	Struct   ElementKind = "STRUCT"   // 对应结构体 (Go, C)
	Enum     ElementKind = "ENUM"     // 对应枚举 (Java, C++, Rust)
	Trait    ElementKind = "TRAIT"    // 对应特质/接口 (Rust, Scala)
	
	// 可执行体
	Function ElementKind = "FUNCTION" // 对应独立函数 (C, Go, JS)
	Method   ElementKind = "METHOD"   // 对应类/结构体的方法
	Macro    ElementKind = "MACRO"    // 对应预处理器宏 (C/C++)
	
	// 存储和声明
	Variable ElementKind = "VARIABLE" // 对应局部/全局变量
	Constant ElementKind = "CONSTANT" // 对应常量
	Field    ElementKind = "FIELD"    // 对应类/结构体/枚举的成员或字段 (Java, Go, C++)
	Type     ElementKind = "TYPE"     // 对应自定义类型别名或基本类型引用
)