package parser

import (
	"os"
	"time"
	
	sitter "github.com/smacker/tree-sitter"
	"github.com/smacker/tree-sitter/parser"
	
	// 在实际项目中，需要引入所有依赖的语言库，并在其 init 函数中调用 RegisterLanguage
	// _ "path/to/go-tree-sitter-bindings"
	// _ "path/to/java-tree-sitter-bindings"
)

// ParserInterface 定义了所有语言解析器的通用能力
type ParserInterface interface {
	// ParseFile 读取文件内容并使用相应的 Tree-sitter 语言库进行解析，返回 AST 根节点。
	ParseFile(filePath string) (*sitter.Node, error)
}

// TreeSitterParser 是 ParserInterface 的具体实现
type TreeSitterParser struct {
	Language Language // 当前解析器针对的语言
	tsParser *sitter.Parser
}

// NewParser 创建一个新的 TreeSitterParser 实例
func NewParser(lang Language) (*TreeSitterParser, error) {
	tsLang, err := GetLanguage(lang)
	if err != nil {
		return nil, err
	}

	tsParser := sitter.NewParser()
	tsParser.SetLanguage(tsLang)

	// 设置超时，防止解析复杂文件时卡死
	tsParser.SetTimeout(5 * time.Second) 

	return &TreeSitterParser{
		Language: lang,
		tsParser: tsParser,
	}, nil
}

// ParseFile 实现了 ParserInterface 接口
func (p *TreeSitterParser) ParseFile(filePath string) (*sitter.Node, error) {
	// 1. 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// 2. 解析文件内容
	tree := p.tsParser.Parse(nil, content)
	
	if tree == nil {
		return nil, fmt.Errorf("tree-sitter failed to parse file %s", filePath)
	}

	// 3. 返回 AST 根节点
	return tree.RootNode(), nil
}

// Close 释放 Tree-sitter 内部资源 (可选)
func (p *TreeSitterParser) Close() {
	if p.tsParser != nil {
		p.tsParser.Close()
	}
}