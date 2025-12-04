package model

// --- 依赖关系类型 (Dependency Relation Types) ---

// DependencyType 是表示依赖关系的字符串常量
type DependencyType string

const (
	Call       DependencyType = "CALL"       // Call: Source calls Target function/method.
	Import     DependencyType = "IMPORT"     // Import: Source file/module imports Target package/module.
	Contain    DependencyType = "CONTAIN"    // Contain: Source element contains Target element (e.g., Class contains Member).
	Parameter  DependencyType = "PARAMETER"  // Parameter: Function/Method uses Target type as a parameter.
	Return     DependencyType = "RETURN"     // Return: Function/Method returns Target type.
	Throw      DependencyType = "THROW"      // Throw: Function/Method throws Target exception type.
	Implement  DependencyType = "IMPLEMENT"  // Implement: Class implements Interface, or function implements Prototype.
	Extend     DependencyType = "EXTEND"     // Extend: Class inherits from another Base Class.
	Create     DependencyType = "CREATE"     // Create: Function/Method instantiates/creates an object of Target type.
	Use        DependencyType = "USE"        // Use: Source expression/block uses Target variable/constant/type (General Reference).
	Cast       DependencyType = "CAST"       // Cast: Expression is explicitly cast to Target type.
	ImplLink   DependencyType = "IMPL_LINK"  // ImplLink: Implementation Link (e.g., C/C++ linking prototype to implementation).
	Annotation DependencyType = "ANNOTATION" // Annotation: Code element is decorated by Target Annotation/Decorator.
	Mixin      DependencyType = "MIXIN"      // Mixin: Class includes/mixes in Target Module (e.g., Ruby).
)

// DependencyRelation 是工具的核心输出结构，描述了 Source 和 Target 之间的一个依赖关系
type DependencyRelation struct {
	Type     DependencyType `json:"Type"`     // Type: 依赖关系的类型 (e.g., CALL, IMPORT, EXTEND)
	Source   *CodeElement   `json:"Source"`   // Source: 关系的发起方（调用者、导入者等）
	Target   *CodeElement   `json:"Target"`   // Target: 关系的指向方（被调用函数、被导入包等）
	Location *Location      `json:"Location"` // Location: 关系发生的代码位置。 对于 Call 关系，这是调用表达式的位置。 对于 Import 关系，这是 import 语句的位置。
	Details  string         `json:"Details"`
}
