package parser

import (
	"fmt"
	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	"io/ioutil"
	"os"
	"path/filepath"

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
func (p *TreeSitterParser) ParseFile(filePath string, enableASTOutput bool) (*sitter.Node, *[]byte, error) {
	// 1. 读取文件内容
	sourceBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// 2. 解析文件内容
	tree := p.tsParser.Parse(sourceBytes, nil)
	if tree == nil {
		return nil, nil, fmt.Errorf("tree-sitter failed to parse file %s", filePath)
	}

	// 3. 输出 AST 文件
	if enableASTOutput {
		if err := writeASTToFile(tree.RootNode(), filePath, sourceBytes); err != nil {
			// 警告而不是失败，不影响主流程
			fmt.Printf("[Warning] Failed to write AST file for %s: %v\n", filePath, err)
		}
	}

	// 4. 返回 AST 根节点
	return tree.RootNode(), &sourceBytes, nil
}

// Close 释放 Tree-sitter 内部资源 (可选)
func (p *TreeSitterParser) Close() {
	if p.tsParser != nil {
		p.tsParser.Close()
	}
}

// writeASTToFile 将 AST 树以 S-expression 格式写入到源码文件同目录下的 .ast 文件中。
func writeASTToFile(rootNode *sitter.Node, filePath string, sourceBytes []byte) error {
	astString := rootNode.ToSexp() // Tree-sitter 的 Node.String() 方法输出 S-expression 格式

	// 构建 AST 文件的输出路径
	dir := filepath.Dir(filePath)
	fileName := filepath.Base(filePath)
	astFileName := fileName + ".ast"
	astFilePath := filepath.Join(dir, astFileName)

	// 写入文件
	return ioutil.WriteFile(astFilePath, []byte(astString), 0644)
}
