package java

import (
	"github.com/CodMac/go-treesitter-dependency-analyzer/collector"
	"github.com/CodMac/go-treesitter-dependency-analyzer/extractor"
	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	"github.com/CodMac/go-treesitter-dependency-analyzer/parser"
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

func init() {
	// 注册 Tree-sitter Java 语言对象
	parser.RegisterLanguage(model.LangJava, sitter.NewLanguage(tree_sitter_java.Language()))
	// 注册 Collector 工厂函数
	collector.RegisterCollector(model.Language("java"), func() collector.Collector {
		return NewJavaCollector()
	})
	// 注册 Extractor 工厂函数
	extractor.RegisterExtractor(model.Language("java"), func() extractor.Extractor {
		return NewJavaExtractor()
	})
}
