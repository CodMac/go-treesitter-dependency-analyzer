package collector

import (
	"fmt"

	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Collector 用于收集符号定义。
type Collector interface {
	// CollectDefinitions 负责遍历 AST，建立并返回该文件的 FileContext。
	CollectDefinitions(rootNode *sitter.Node, filePath string, sourceBytes *[]byte) (*model.FileContext, error)
}

// LanguageCollectorFactory 是一个工厂函数类型，用于创建特定语言的 Collector 实例。
type LanguageCollectorFactory func() Collector

var collectorFactories = make(map[model.Language]LanguageCollectorFactory)

// RegisterCollector 注册一个语言与其对应的 Collector 工厂函数。
func RegisterCollector(lang model.Language, factory LanguageCollectorFactory) {
	collectorFactories[lang] = factory
}

// GetCollector 根据语言类型获取对应的 Collector 实例。
func GetCollector(lang model.Language) (Collector, error) {
	factory, ok := collectorFactories[lang]
	if !ok {
		return nil, fmt.Errorf("no collector registered for language: %s", lang)
	}

	return factory(), nil
}
