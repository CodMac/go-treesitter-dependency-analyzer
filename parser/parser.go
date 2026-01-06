package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CodMac/go-treesitter-dependency-analyzer/model"

	sitter "github.com/tree-sitter/go-tree-sitter"
	// 导入所有语言绑定，确保 GetLanguage 可以找到
	_ "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

// Parser 定义了所有语言解析器的通用能力
type Parser interface {
	// ParseFile 的签名需要更新以适应配置参数
	ParseFile(filePath string, enableASTOutput bool, formatAST bool) (*sitter.Node, *[]byte, error)
	Close()
}

// TreeSitterParser Parser的具体实现
type TreeSitterParser struct {
	Language model.Language // 当前解析器针对的语言
	tsParser *sitter.Parser
}

// NewParser 创建一个新的 TreeSitterParser 实例
func NewParser(lang model.Language) (Parser, error) {
	tsLang, err := model.GetLanguage(lang)
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
// 接受 enableASTOutput 和 formatAST 两个布尔参数
func (p *TreeSitterParser) ParseFile(filePath string, enableASTOutput bool, formatAST bool) (*sitter.Node, *[]byte, error) {
	// 1. 读取文件内容
	sourceBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// 2. 解析文件内容
	// 假设使用 UTF8 编码
	tree := p.tsParser.Parse(sourceBytes, nil)
	if tree == nil {
		return nil, nil, fmt.Errorf("tree-sitter failed to parse file %s", filePath)
	}

	// 3. 输出 AST 文件 (根据 enableASTOutput 和 formatAST 决定行为)
	if enableASTOutput {
		if err := writeASTToFile(tree.RootNode(), filePath, sourceBytes, formatAST); err != nil {
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

// writeASTToFile 根据 formatFlag 决定使用紧凑或格式化的 S-expression
func writeASTToFile(rootNode *sitter.Node, filePath string, sourceBytes []byte, formatAST bool) error {
	var astString string

	// 是否格式化
	if formatAST {
		astString = formatSExpression(rootNode, sourceBytes, 0) // 使用格式化函数（缩进和换行）
	} else {
		astString = rootNode.ToSexp()
	}

	// 构建 AST 文件的输出路径
	dir := filepath.Dir(filePath)
	fileName := filepath.Base(filePath)
	astFileName := fileName + ".ast"
	if formatAST {
		astFileName += ".format"
	}
	astFilePath := filepath.Join(dir, astFileName)

	// 写入文件
	return os.WriteFile(astFilePath, []byte(astString), 0644)
}

// formatSExpression 递归遍历抽象语法树（AST）, 并生成格式化的S-expression字符串。
func formatSExpression(node *sitter.Node, sourceCode []byte, indentLevel int) string {
	indent := strings.Repeat("  ", indentLevel)

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("%s(%s", indent, node.Kind()))

	// 不存在child的话，需要从sourceCode中提取content
	if node.NamedChildCount() == 0 {
		start := node.StartByte()
		end := node.EndByte()

		var content string
		if start < end && int(end) <= len(sourceCode) {
			content = string(sourceCode[start:end])
		} else {
			content = ""
		}

		trimmedContent := strings.TrimSpace(content)
		if trimmedContent != "" && !isPunctuation(trimmedContent) {
			builder.WriteString(fmt.Sprintf(" %q)", trimmedContent))
		} else {
			builder.WriteString(")")
		}
		return builder.String()
	}

	// Process non-leaf nodes
	builder.WriteString("\n")
	child := node.NamedChild(0)
	for child != nil {
		builder.WriteString(formatSExpression(child, sourceCode, indentLevel+1))
		builder.WriteString("\n")

		child = child.NextNamedSibling()
	}

	// Close the node
	result := builder.String()
	result = strings.TrimSuffix(result, "\n")

	builder.Reset()
	builder.WriteString(result)
	builder.WriteString(fmt.Sprintf("\n%s)", indent))

	return builder.String()
}

// isPunctuation 检查内容是否可能只是标点符号
func isPunctuation(s string) bool {
	for _, char := range s {
		if !strings.ContainsRune("(){}[];,\"'`", char) {
			return false
		}
	}
	return true
}
