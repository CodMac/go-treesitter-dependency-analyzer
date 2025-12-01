package model

// DependencyRelation 是工具的核心输出结构，描述了 Source 和 Target 之间的一个依赖关系
type DependencyRelation struct {
	// Type: 依赖关系的类型 (e.g., CALL, IMPORT, EXTEND)
	Type DependencyType `json:"Type"`

	// Source: 关系的发起方（调用者、导入者等）
	Source *CodeElement `json:"Source"`

	// Target: 关系的指向方（被调用函数、被导入包等）
	Target *CodeElement `json:"Target"`

	// Location: 关系发生的代码位置。
	// 对于 Call 关系，这是调用表达式的位置。
	// 对于 Import 关系，这是 import 语句的位置。
	Location *Location `json:"Location"`
}