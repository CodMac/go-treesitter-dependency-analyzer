package extractor

import (
	"fmt"
	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Extractor 用于提取关系，需要全局上下文。
type Extractor interface {
	// Extract 接收 AST 根节点、文件路径和全局上下文，返回依赖关系。
	Extract(rootNode *sitter.Node, filePath string, gCtx *model.GlobalContext) ([]*model.DependencyRelation, error)
}

// LanguageExtractorFactory 是一个工厂函数类型，用于创建特定语言的 Extractor 实例。
type LanguageExtractorFactory func() Extractor

var extractorFactories = make(map[model.Language]LanguageExtractorFactory)

// RegisterExtractor 注册一个语言与其对应的 Extractor 工厂函数。
func RegisterExtractor(lang model.Language, factory LanguageExtractorFactory) {
	extractorFactories[lang] = factory
}

// GetExtractor 根据语言类型获取对应的 Extractor 实例。
func GetExtractor(lang model.Language) (Extractor, error) {
	factory, ok := extractorFactories[lang]
	if !ok {
		return nil, fmt.Errorf("no extractor registered for language: %s", lang)
	}
	return factory(), nil
}
