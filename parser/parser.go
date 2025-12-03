package parser

import (
	"fmt"
	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	"os"

	sitter "github.com/tree-sitter/go-tree-sitter"
	_ "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

// Parser 定义了所有语言解析器的通用能力
type Parser interface {
	ParseFile(filePath string) (*sitter.Node, error) // ParseFile 读取文件内容并使用相应的 Tree-sitter 语言库进行解析，返回 AST 根节点。
}

// TreeSitterParser Parser的具体实现
type TreeSitterParser struct {
	Language model.Language // 当前解析器针对的语言
	tsParser *sitter.Parser
}

// NewParser 创建一个新的 TreeSitterParser 实例
func NewParser(lang model.Language) (*TreeSitterParser, error) {
	tsLang, err := GetLanguage(lang)
	if err != nil {
		return nil, err
	}

	tsParser := sitter.NewParser()
	tsParser.SetLanguage(tsLang)

	return &TreeSitterParser{
		Language: lang,
		tsParser: tsParser,
	}, nil
}

// ParseFile 实现了 ParserInterface 接口
func (p *TreeSitterParser) ParseFile(filePath string) (*sitter.Node, *[]byte, error) {
	// 1. 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// 2. 解析文件内容
	tree := p.tsParser.Parse(content, nil)

	if tree == nil {
		return nil, nil, fmt.Errorf("tree-sitter failed to parse file %s", filePath)
	}

	// 3. 返回 AST 根节点
	return tree.RootNode(), &content, nil
}

// Close 释放 Tree-sitter 内部资源 (可选)
func (p *TreeSitterParser) Close() {
	if p.tsParser != nil {
		p.tsParser.Close()
	}
}
