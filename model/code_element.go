package model

// Location 描述了代码元素或依赖关系在源码中的位置
type Location struct {
	FilePath string `json:"FilePath"`
	StartLine int   `json:"StartLine"`
	EndLine int     `json:"EndLine"`
	StartColumn int `json:"StartColumn"`
	EndColumn int   `json:"EndColumn"`
}

// CodeElement 描述了源码中的一个可识别实体（Source 或 Target）
type CodeElement struct {
	// Kind: 元素的类型 (e.g., FUNCTION, CLASS, VARIABLE)
	Kind ElementKind `json:"Kind"`

	// Name: 元素的短名称 (e.g., "main", "CalculateSum")
	Name string `json:"Name"`

	// QualifiedName: 元素的完整限定名称 (e.g., "pkg/util.Utility.CalculateSum")
	QualifiedName string `json:"QualifiedName"`

	// Path: 元素所在的文件路径 (相对于项目根目录)
	Path string `json:"Path"`

	// Signature: 元素的完整签名（针对函数/方法，包含参数和返回值类型）
	Signature string `json:"Signature,omitempty"` 
	
	// StartLocation: 元素的起始位置
	StartLocation *Location `json:"StartLocation,omitempty"`
	
	// EndLocation: 元素的结束位置
	EndLocation *Location `json:"EndLocation,omitempty"`
}