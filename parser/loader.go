package parser

import (
	sitter "github.com/smacker/tree-sitter"
	"fmt"
)

// Language 标识支持的编程语言
type Language string

const (
	LangGo       Language = "go"
	LangJava     Language = "java"
	// LangPython   Language = "python"
	// LangC      Language = "c"
	// LangRust   Language = "rust"
	// ... 更多语言
)

// langMap 存储语言标识到 Tree-sitter 语言对象的映射
var langMap = make(map[Language]*sitter.Language)

// RegisterLanguage 用于注册 Tree-sitter 语言库
// 外部调用方（如 main.go 或特定语言包的 init 函数）应调用此函数
func RegisterLanguage(lang Language, tsLang *sitter.Language) {
	langMap[lang] = tsLang
}

// GetLanguage 获取已注册的 Tree-sitter 语言对象
func GetLanguage(lang Language) (*sitter.Language, error) {
	tsLang, ok := langMap[lang]
	if !ok {
		return nil, fmt.Errorf("language %s not registered", lang)
	}
	return tsLang, nil
}

// init 模拟注册 Java 语言库。在实际项目中，您需要引入相应的 Tree-sitter Go 绑定库，
// 并在该库的 init 函数中调用 RegisterLanguage。
/*
func init() {
    // 假设引入了 tree-sitter-java 的 Go 绑定库
    // import tree_sitter_java "path/to/tree-sitter-java-go"
    // RegisterLanguage(LangJava, tree_sitter_java.Language())
}
*/