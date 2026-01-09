package java

import (
	"github.com/CodMac/go-treesitter-dependency-analyzer/core"

	sitter "github.com/tree-sitter/go-tree-sitter"

	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

func init() {
	// 注册 Tree-sitter Java 语言对象
	core.RegisterLanguage(core.LangJava, sitter.NewLanguage(tree_sitter_java.Language()))

	// 注册 NoiseFilter(噪音过滤)
	core.RegisterNoiseFilter(core.LangJava, NewJavaNoiseFilter())

	// 注册 SymbolResolver(符号解析)
	core.RegisterSymbolResolver(core.LangJava, NewJavaSymbolResolver())

	// 注册 Collector
	core.RegisterCollector(core.LangJava, NewJavaCollector())

	// 注册 Extractor
	//core.RegisterExtractor(core.LangJava, NewJavaExtractor())
}
