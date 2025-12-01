package extractor

import (
	sitter "github.com/smacker/tree-sitter"
	"go-treesitter-dependency-analyzer/model"
)

// DefinitionCollector 定义了第一阶段收集定义的能力。
type DefinitionCollector interface {
	// CollectDefinitions 仅负责遍历 AST，建立并返回该文件的 FileContext。
	CollectDefinitions(rootNode *sitter.Node, filePath string) (*model.FileContext, error)
}

// ContextExtractor 定义了第二阶段提取关系的能力，需要全局上下文。
type ContextExtractor interface {
	// Extract 接收 AST 根节点、文件路径和全局上下文，返回依赖关系。
	Extract(rootNode *sitter.Node, filePath string, globalContext *model.GlobalContext) ([]*model.DependencyRelation, error)
}

// Extractor 是一个组合接口，要求所有适配器同时实现这两阶段的能力。
type Extractor interface {
	DefinitionCollector
	ContextExtractor
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