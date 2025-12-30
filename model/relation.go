package model

// --- 依赖关系类型 (Dependency Relation Types) ---

// DependencyType 是表示依赖关系的字符串常量
type DependencyType string

const (
	// Import 导入 imports Target package/module
	// e.g., [Java: Source(File) -> Target(Package/Class/Constant)]
	Import DependencyType = "IMPORT"
	// Contain 包含成员 Source element contains Target element
	// e.g., [Java: Source(Class...) -> Target(Class/Interface/Enum/Method/Field...)]
	Contain DependencyType = "CONTAIN"
	// Parameter 函数参数
	// e.g., [Java: Source(Method) -> Target(Type/Class)]
	Parameter DependencyType = "PARAMETER"
	// Return 函数返回类型
	// e.g., [Java: Source(Method) -> Target(Type/Class)]
	Return DependencyType = "RETURN"
	// Throw 函数抛出类型
	// e.g., [Java: Source(Method) -> Target(Class)]
	Throw DependencyType = "THROW"
	// Implement 类实现
	// e.g., [Java: Source(Class) -> Target(Interface)].
	Implement DependencyType = "IMPLEMENT"
	// Extend 类/接口继承
	// e.g., [Java: Source(Class) -> Target(Class)]、[Java: Source(Interface) -> Target(Interface)]
	Extend DependencyType = "EXTEND"
	// Annotation 注解修饰
	// e.g., [Java: Source(Class...) -> Target(KAnnotation)]
	Annotation DependencyType = "ANNOTATION"
	// Call 函数调用 calls a Target Method/Function
	// 关注点：谁调用了谁
	// e.g., [Java: Source(Method) -> Target(Method)]
	Call DependencyType = "CALL"
	// Use 字段使用 Source expression/block uses Target field/variable/constant/type
	// 关注点：谁使用了谁
	// e.g., [Java: Source(Method) -> Target(Field)]
	Use DependencyType = "USE"
	// Create 显式实例创建 creates an object of Target type
	// 关注点：哪个地方存在实例创建
	// e.g., [Java: Source(Method) -> Target(Class)]
	Create DependencyType = "CREATE"
	// Cast 类型强转 Target type explicitly cast to Target type
	// 关注点：有哪些类型被强行转换
	// e.g., [Java: Source(Class) -> Target(Class)]
	Cast     DependencyType = "CAST"
	ImplLink DependencyType = "IMPL_LINK" // ImplLink: Implementation Link (e.g., C/C++ linking prototype to implementation).
	Mixin    DependencyType = "MIXIN"     // Mixin: Class includes/mixes in Target Module (e.g., Ruby).
)

// DependencyRelation 是工具的核心输出结构，描述了 Source 和 Target 之间的一个依赖关系
type DependencyRelation struct {
	Type     DependencyType `json:"Type"`     // Type: 依赖关系的类型 (e.g., CALL, IMPORT, EXTEND)
	Source   *CodeElement   `json:"Source"`   // Source: 关系的发起方（调用者、导入者等）
	Target   *CodeElement   `json:"Target"`   // Target: 关系的指向方（被调用函数、被导入包等）
	Location *Location      `json:"Location"` // Location: 关系发生的代码位置。 对于 Call 关系，这是调用表达式的位置。 对于 Import 关系，这是 import 语句的位置。
	Details  string         `json:"Details"`
}
