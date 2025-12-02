package parser

import (
	"fmt"
	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// langMap 存储语言标识到 Tree-sitter 语言对象的映射
var langMap = make(map[model.Language]*sitter.Language)

// RegisterLanguage 用于注册 Tree-sitter 语言库
func RegisterLanguage(lang model.Language, tsLang *sitter.Language) {
	langMap[lang] = tsLang
}

// GetLanguage 获取已注册的 Tree-sitter 语言对象
func GetLanguage(lang model.Language) (*sitter.Language, error) {
	tsLang, ok := langMap[lang]
	if !ok {
		return nil, fmt.Errorf("language %s not registered", lang)
	}

	return tsLang, nil
}
