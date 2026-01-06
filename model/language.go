package model

import (
	"fmt"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Language 标识支持的编程语言
type Language string

const (
	LangGo   Language = "go"
	LangJava Language = "java"
)

// langMap 存储语言标识到 Tree-sitter 语言对象的映射
var langMap = make(map[Language]*sitter.Language)

// RegisterLanguage 用于注册 Tree-sitter 语言库
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
