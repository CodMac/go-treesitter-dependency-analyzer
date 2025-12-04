package main_test

import (
	"context"
	"testing"

	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	"github.com/CodMac/go-treesitter-dependency-analyzer/processor"
	_ "github.com/CodMac/go-treesitter-dependency-analyzer/x/java" // 确保注册 Java
)

func TestFileProcessor_ProcessFiles_Java(t *testing.T) {
	userPath := getTestFilePath("User.java")
	servicePath := getTestFilePath("UserService.java")

	filePaths := []string{userPath, servicePath}

	// 1. 初始化处理器
	proc := processor.NewFileProcessor(model.LangJava, true, true, 4)

	// 2. 运行两阶段处理
	ctx := context.Background()
	relations, err := proc.ProcessFiles(ctx, filePaths)

	if err != nil {
		t.Fatalf("Processor failed to process files: %v", err)
	}

	// 3. 验证结果
	if len(relations) < 8 {
		t.Errorf("Expected at least 8 dependency relations (2 imports, 1 create, 1 return, 1 throw, 2 calls, 1 parameter), got %d", len(relations))
	}

	// 预期关键关系检查
	expectedRelations := map[model.DependencyType]map[string]bool{
		model.Import: {"com.example.model.User": false, "java.util.List": false, "java.io.IOException": false},
		model.Create: {"com.example.model.User": false}, // UserService.createNewUser 创建 User
		model.Return: {"com.example.model.User": false}, // UserService.createNewUser 返回 User
		model.Throw:  {"java.io.IOException": false},    // UserService.createNewUser 抛出 IOException
		model.Call:   {"System.out.println": false},     // UserService.processUsers 调用
	}

	for _, rel := range relations {
		if targets, ok := expectedRelations[rel.Type]; ok {
			qn := rel.Target.QualifiedName
			if rel.Type == model.Import {
				// 对于 Import, QN 是完整的包/类名
				if _, found := targets[qn]; found {
					targets[qn] = true
				}
			} else if rel.Type == model.Create && rel.Target.Name == "User" {
				// CREATE 目标是 User 类
				targets["com.example.model.User"] = true
			} else if rel.Type == model.Return && rel.Target.Name == "User" {
				targets["com.example.model.User"] = true
			} else if rel.Type == model.Throw && rel.Target.Name == "IOException" {
				targets["java.io.IOException"] = true
			} else if rel.Type == model.Call && rel.Target.Name == "println" {
				targets["System.out.println"] = true
			}
		}
	}

	// 最终验证所有关键关系是否都找到
	allFound := true
	for dtType, targets := range expectedRelations {
		for target, found := range targets {
			if !found {
				t.Errorf("Missing expected dependency type %s to target %s", dtType, target)
				allFound = false
			}
		}
	}

	if allFound {
		t.Logf("Successfully found all %d critical dependencies.", len(expectedRelations))
	}
}
