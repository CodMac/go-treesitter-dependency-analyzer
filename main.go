package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	
	"go-treesitter-dependency-analyzer/model"
	"go-treesitter-dependency-analyzer/output"
	"go-treesitter-dependency-analyzer/processor"
	
	// 导入所有语言的实现，以触发其 init() 函数注册 Extractor 和 Language
	_ "go-treesitter-dependency-analyzer/extractor/java" // 确保 Java Extractor 被注册
	_ "go-treesitter-dependency-analyzer/extractor/golang"
	
	// 关键：导入 Tree-sitter 语言绑定库，触发其 init 函数，该函数会调用 parser.RegisterLanguage
	java_ts "github.com/tree-sitter/tree-sitter-java"
	_ "github.com/smacker/tree-sitter-go"
)

var (
	inputPath string
	language  string
	workers   int
)

func init() {
	// 命令行参数定义
	flag.StringVar(&inputPath, "path", ".", "要分析的源码目录或文件路径")
	flag.StringVar(&language, "lang", "java", "要分析的编程语言 (e.g., go, java, python)")
	flag.IntVar(&workers, "workers", runtime.NumCPU(), "并发处理文件的协程数量 (默认 CPU 核心数)")

	// 模拟注册 Java 语言（实际应由 Tree-sitter 绑定库的 init 函数完成）
	// TODO: 在集成 Tree-sitter 绑定库后删除此模拟注册
	// 假设 tree-sitter-java-go 提供了 Language() 函数
	// parser.RegisterLanguage(parser.LangJava, tree_sitter_java_go.Language()) 
}

func main() {
	flag.Parse()

	// 1. 验证输入语言
	lang := model.Language(language)
	if _, err := extractor.GetExtractor(lang); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Unsupported language or extractor not registered: %s\n", language)
		os.Exit(1)
	}

	// 2. 查找所有要分析的文件
	filePaths, err := discoverFiles(inputPath, lang)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error discovering files: %v\n", err)
		os.Exit(1)
	}
	if len(filePaths) == 0 {
		fmt.Println("No source files found to analyze.")
		return
	}

	fmt.Printf("Starting analysis on %d files using %s with %d workers...\n", len(filePaths), lang, workers)

	// 3. 启动处理器
	proc := processor.NewFileProcessor(lang, workers)
	ctx := context.Background()
	
	relations, err := proc.ProcessFiles(ctx, filePaths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Fatal processing error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Analysis complete. Found %d dependency relations.\n", len(relations))

	// 4. 输出结果
	jsonlWriter := output.NewJSONLWriter(os.Stdout)
	if err := jsonlWriter.WriteAllRelations(relations); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
		os.Exit(1)
	}
}

// discoverFiles 递归查找目录下所有符合语言要求的文件路径。
func discoverFiles(root string, lang model.Language) ([]string, error) {
	var files []string
	ext := getFileExtension(lang)

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// 忽略隐藏目录和文件
		if d.IsDir() && (d.Name() == "." || d.Name() == ".." || d.Name()[0] == '.') {
			return filepath.SkipDir
		}
		
		if !d.IsDir() && filepath.Ext(path) == ext {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// getFileExtension 辅助函数，根据语言返回文件扩展名
func getFileExtension(lang model.Language) string {
	switch lang {
	case model.Language("go"):
		return ".go"
	case model.Language("java"):
		return ".java"
	case model.Language("python"):
		return ".py"
	// TODO: 完善其他语言
	default:
		return ""
	}
}