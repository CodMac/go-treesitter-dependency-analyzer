package main

import (
	"context"
	"fmt"
	"os"

	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	"github.com/CodMac/go-treesitter-dependency-analyzer/output"
	"github.com/CodMac/go-treesitter-dependency-analyzer/processor"

	// 导入所有语言包，以触发它们的 init() 函数进行注册
	_ "github.com/CodMac/go-treesitter-dependency-analyzer/x/java" // 确保注册 Java Collector/Extractor
)

func main() {
	// 命令行参数处理 (简化版)
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <language> <file1> [<file2>...]\n", os.Args[0])
		os.Exit(1)
	}

	lang := model.Language(os.Args[1])
	filePaths := os.Args[2:]

	// 1. 初始化处理器
	// 假设使用 4 个工作协程
	proc := processor.NewFileProcessor(lang, 4)

	// 2. 运行两阶段处理
	ctx := context.Background()
	relations, err := proc.ProcessFiles(ctx, filePaths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error processing files: %v\n", err)
		os.Exit(1)
	}

	// 3. 结果输出 (JSONL格式到标准输出)
	writer := output.NewJSONLWriter(os.Stdout)
	if err := writer.WriteAllRelations(relations); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "\nSuccessfully extracted %d dependencies.\n", len(relations))
}
