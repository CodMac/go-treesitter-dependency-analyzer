package main_test

import (
	"context"
	"github.com/CodMac/go-treesitter-dependency-analyzer/processor"
	"os"
	"path/filepath"
	"testing"

	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	_ "github.com/CodMac/go-treesitter-dependency-analyzer/x/java" // 确保注册了 Java 处理器
)

func TestFileProcessor_ProcessFiles(t *testing.T) {
	// 1. 准备测试数据：创建临时项目结构
	tmpDir, err := os.MkdirTemp("", "analyzer-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// 文件 A: 定义 Base 类
	fileA := filepath.Join(tmpDir, "Base.java")
	codeA := `package com.test; public class Base { public void hello() {} }`

	// 文件 B: 继承 Base 并调用
	fileB := filepath.Join(tmpDir, "App.java")
	codeB := `package com.test;
       import com.test.Base;
       public class App extends Base {
         public void run() { new Base().hello(); }
       }`

	os.WriteFile(fileA, []byte(codeA), 0644)
	os.WriteFile(fileB, []byte(codeB), 0644)

	// 2. 初始化 Processor
	fp := processor.NewFileProcessor(model.LangJava, false, false, 2)

	// 3. 执行分析
	filePaths := []string{fileA, fileB}
	rels, gCtx, err := fp.ProcessFiles(context.Background(), tmpDir, filePaths)
	if err != nil {
		t.Fatalf("ProcessFiles failed: %v", err)
	}

	// 4. 验证层级关系 (Stage 2 的产物)
	t.Run("VerifyHierarchy", func(t *testing.T) {
		hasPackage := false
		hasFileInPkg := false
		for _, rel := range rels {
			if rel.Type == "CONTAINS" {
				if rel.Source.Kind == model.Package && rel.Source.QualifiedName == "com.test" {
					hasPackage = true
				}
				// 检查包是否包含文件 (注意：由于 path 已经归一化，Target 应为相对路径)
				if rel.Source.QualifiedName == "com.test" && rel.Target.Kind == model.File {
					hasFileInPkg = true
				}
			}
		}
		if !hasPackage {
			t.Error("Missing package 'com.test' in hierarchy")
		}
		if !hasFileInPkg {
			t.Error("Missing Package -> File relationship")
		}
	})

	// 5. 验证逻辑依赖关系 (Stage 3 的产物)
	t.Run("VerifyLogicDependencies", func(t *testing.T) {
		hasInherit := false
		hasCall := false
		for _, rel := range rels {
			// 验证 App 继承 Base
			if rel.Type == model.Extend && rel.Source.Name == "App" && rel.Target.Name == "Base" {
				hasInherit = true
			}
			// 验证方法调用
			if rel.Type == model.Call && rel.Target.Name == "hello" {
				hasCall = true
			}
		}
		if !hasInherit {
			t.Error("Failed to detect inheritance: App extends Base")
		}
		if !hasCall {
			t.Error("Failed to detect method call: hello()")
		}
	})

	// 6. 验证全局上下文索引
	t.Run("VerifyGlobalContext", func(t *testing.T) {
		if _, ok := gCtx.DefinitionsByQN["com.test.Base"]; !ok {
			t.Error("GlobalContext missing index for com.test.Base")
		}
	})
}
