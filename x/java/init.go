package java

import (
	"github.com/CodMac/go-treesitter-dependency-analyzer/collector"
	"github.com/CodMac/go-treesitter-dependency-analyzer/context"
	"github.com/CodMac/go-treesitter-dependency-analyzer/extractor"
	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	"github.com/CodMac/go-treesitter-dependency-analyzer/noisefilter"
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

func init() {
	// 注册 Tree-sitter Java 语言对象
	model.RegisterLanguage(model.LangJava, sitter.NewLanguage(tree_sitter_java.Language()))
	// 注册 Collector
	collector.RegisterCollector(model.LangJava, NewJavaCollector())
	// 注册 Extractor
	extractor.RegisterExtractor(model.LangJava, NewJavaExtractor())
	// 注册 NoiseFilter(噪音过滤)
	noisefilter.RegisterNoiseFilter(model.LangJava, NewJavaNoiseFilter())
	// 注册 SymbolResolver(符号解析)
	context.RegisterSymbolResolver(model.LangJava, NewJavaSymbolResolver())
}
