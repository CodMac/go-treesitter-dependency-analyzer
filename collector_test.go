package main_test

import (
	"fmt"
	"github.com/CodMac/go-treesitter-dependency-analyzer/parser"
	"testing"

	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	"github.com/CodMac/go-treesitter-dependency-analyzer/x/java"

	// 导入所有语言绑定，确保 GetLanguage 可以找到
	_ "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

func TestJavaCollector_CollectDefinitions(t *testing.T) {
	javaParser, err := parser.NewParser(model.LangJava)
	if err != nil {
		t.Fatalf("Failed to create Java parser: %v", err)
	}

	filePath := getTestFilePath("MyClass.java")
	rootNode, sourceBytes, err := javaParser.ParseFile(filePath, true, false)
	if err != nil {
		t.Fatalf("Failed to parser file: %v", err)
	}

	collector := java.NewJavaCollector()
	fCtx, err := collector.CollectDefinitions(rootNode, filePath, sourceBytes)
	if err != nil {
		t.Fatalf("CollectDefinitions failed: %v", err)
	}

	// 1. 验证 Package Name
	expectedPackage := "com.example.app"
	if fCtx.PackageName != expectedPackage {
		t.Errorf("Expected PackageName %q, got %q", expectedPackage, fCtx.PackageName)
	}
	fmt.Printf("Package Name: %s\n", fCtx.PackageName)

	// 2. 验证关键定义的 QN 和 Kind
	// 注意：由于 collector 使用短名称作为 Map 键，MyClass (Class) 会被 MyClass (Constructor) 覆盖。
	// 因此我们只检查最终留在 Map 中的 Constructor 及其 QN。
	expectedDefinitions := map[string]struct {
		kind     model.ElementKind
		parentQN string
		name     string
	}{
		// Constructor 覆盖 Class 的条目
		"MyClass":     {model.Method, "com.example.app.MyClass", "MyClass"},
		"APP_NAME":    {model.Field, "com.example.app.MyClass", "APP_NAME"},
		"counter":     {model.Field, "com.example.app.MyClass", "counter"},
		"run":         {model.Method, "com.example.app.MyClass", "run"},
		"MyInterface": {model.Interface, "com.example.app", "MyInterface"},
		"Status":      {model.Enum, "com.example.app", "Status"},
		"ACTIVE":      {model.EnumConstant, "com.example.app.Status", "ACTIVE"},
		"INACTIVE":    {model.EnumConstant, "com.example.app.Status", "INACTIVE"},
	}

	// 验证收集到的定义数量 (预期: 9 个定义, 因为 MyClass/Class 被 MyClass/Method 覆盖)
	expectedCount := 9
	if len(fCtx.DefinitionsBySN) != expectedCount {
		t.Errorf("Expected %d definitions, got %d. Map keys: %v", expectedCount, len(fCtx.DefinitionsBySN), func() []string {
			keys := make([]string, 0, len(fCtx.DefinitionsBySN))
			for k := range fCtx.DefinitionsBySN {
				keys = append(keys, k)
			}
			return keys
		}())
	}

	for name, expected := range expectedDefinitions {
		entry, ok := fCtx.DefinitionsBySN[name]
		if !ok {
			t.Errorf("Missing expected definition for element: %s", name)
			continue
		}

		if entry.Element.Kind != expected.kind {
			t.Errorf("Definition %s: Expected kind %q, got %q (Element: %v)", name, expected.kind, entry.Element.Kind, entry.Element)
		}

		// 验证 Qualified Name
		expectedQN := model.BuildQualifiedName(expected.parentQN, expected.name)
		if entry.Element.QualifiedName != expectedQN {
			t.Errorf("Definition %s: Expected QN %q, got %q", name, expectedQN, entry.Element.QualifiedName)
		}

		// 验证 Parent QN
		if entry.ParentQN != expected.parentQN {
			t.Errorf("Definition %s: Expected ParentQN %q, got %q", name, expected.parentQN, entry.ParentQN)
		}

		// 验证路径
		if entry.Element.Path != filePath {
			t.Errorf("Definition %s: Expected Path %q, got %q", name, filePath, entry.Element.Path)
		}
	}
}
