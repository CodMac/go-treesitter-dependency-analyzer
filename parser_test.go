package main_test

import (
	sitter "github.com/tree-sitter/go-tree-sitter"
	"path/filepath"
	"testing"

	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	"github.com/CodMac/go-treesitter-dependency-analyzer/parser"
	_ "github.com/CodMac/go-treesitter-dependency-analyzer/x/java" // 确保注册 Java 语言
)

// getTestFilePath 辅助函数，用于获取测试文件路径
func getTestFilePath(name string) string {
	currentDir, _ := filepath.Abs(filepath.Dir("."))
	return filepath.Join(currentDir, "x", "java", "testdata", name)
}

func TestTreeSitterParser_ParseFile(t *testing.T) {
	// 1. 尝试获取并初始化 Java 解析器
	javaParser, err := parser.NewParser(model.LangJava)
	if err != nil {
		t.Fatalf("Failed to create Java parser: %v", err)
	}
	defer javaParser.Close()

	// 2. 尝试解析一个 Java 文件
	filePath := getTestFilePath("UserService.java")
	rootNode, sourceBytes, err := javaParser.ParseFile(filePath, true)

	if err != nil {
		t.Fatalf("ParseFile failed for %s: %v", filePath, err)
	}
	if rootNode == nil {
		t.Fatal("RootNode is nil after parsing")
	}
	if sourceBytes == nil || len(*sourceBytes) == 0 {
		t.Fatal("SourceBytes is nil or empty after parsing")
	}

	// 3. 验证根节点类型和内容
	// Tree-sitter Java 文件的根节点类型是 "program"
	if rootNode.Kind() != "program" {
		t.Errorf("Expected root node kind 'program', got '%s'", rootNode.Kind())
	}

	// 4. 验证 AST 结构完整性（例如，检查是否有重要的子节点）
	// 查找 class_declaration
	var classNode *sitter.Node
	cursor := rootNode.Walk()
	if cursor.GotoFirstChild() {
		for {
			node := cursor.Node()
			if node.Kind() == "class_declaration" {
				classNode = node
				break
			}
			if !cursor.GotoNextSibling() {
				break
			}
		}
	}
	cursor.Close()

	if classNode == nil {
		t.Fatal("Could not find 'class_declaration' node.")
	}

	classNameNode := classNode.ChildByFieldName("name")
	if classNameNode == nil || classNameNode.Kind() != "identifier" || classNameNode.Utf8Text(*sourceBytes) != "UserService" {
		t.Errorf("Expected class name 'UserService', got '%s'", classNameNode.Utf8Text(*sourceBytes))
	}
}
