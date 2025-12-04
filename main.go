package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	"github.com/CodMac/go-treesitter-dependency-analyzer/output"
	"github.com/CodMac/go-treesitter-dependency-analyzer/processor"
	// 导入所有语言包，以触发它们的 init() 函数进行注册
	_ "github.com/CodMac/go-treesitter-dependency-analyzer/x/java"
)

// Config 存储所有命令行配置选项
type Config struct {
	Language        model.Language
	ProjectPath     string
	FileFilterRegex string
	OutputAST       bool
	FormatAST       bool
	Concurrency     int
}

func main() {
	var cfg Config

	// 1. 定义命令行参数
	flag.StringVar((*string)(&cfg.Language), "lang", "go", "指定要分析的编程语言 (例如: go, java, python)")
	flag.StringVar(&cfg.ProjectPath, "path", ".", "要分析的源代码文件或目录路径")
	flag.StringVar(&cfg.FileFilterRegex, "filter", "", "用于过滤文件的正则表达式 (例如: \".*\\.go$\")")
	flag.BoolVar(&cfg.OutputAST, "output-ast", false, "是否将 AST S-expression 输出到 .ast 文件")
	flag.BoolVar(&cfg.FormatAST, "format-ast", true, "是否格式化输出 AST S-expression (仅在 --output-ast 启用时有效)")
	flag.IntVar(&cfg.Concurrency, "jobs", 4, "并发处理文件的协程数量")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [OPTIONS]\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "A tool for analyzing source code dependencies using Tree-sitter.\n\n")
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
	}

	flag.Parse()

	if cfg.ProjectPath == "" {
		fmt.Fprintln(os.Stderr, "Error: The --path flag is required.")
		os.Exit(1)
	}

	// 2. 文件查找和过滤
	filePaths, err := findFiles(cfg.ProjectPath, cfg.FileFilterRegex)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding files: %v\n", err)
		os.Exit(1)
	}
	if len(filePaths) == 0 {
		fmt.Fprintf(os.Stderr, "Warning: No files found in %s matching filter %q.\n", cfg.ProjectPath, cfg.FileFilterRegex)
		os.Exit(0)
	}
	fmt.Fprintf(os.Stderr, "Found %d files to analyze.\n", len(filePaths))

	// 3. 初始化处理器 (需要更新 processor.NewFileProcessor 的签名以接收配置)
	// 假设 processor 包已被更新为接受完整的配置结构
	proc := processor.NewFileProcessor(cfg.Language, cfg.OutputAST, cfg.FormatAST, cfg.Concurrency)

	// 4. 运行两阶段处理
	ctx := context.Background()
	relations, err := proc.ProcessFiles(ctx, filePaths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error processing files: %v\n", err)
		os.Exit(1)
	}

	// 5. 结果输出 (JSONL格式到标准输出)
	writer := output.NewJSONLWriter(os.Stdout)
	if err := writer.WriteAllRelations(relations); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "\nSuccessfully extracted %d dependencies.\n", len(relations))
}

// findFiles 递归查找目录下的所有文件，并应用正则表达式过滤。
// 如果 path 是单个文件，则直接返回该文件路径。
func findFiles(path string, regex string) ([]string, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	// 如果是单个文件，且匹配过滤器，则返回
	if !fileInfo.IsDir() {
		if regex == "" || regexp.MustCompile(regex).MatchString(filepath.Base(path)) {
			return []string{path}, nil
		}
		return []string{}, nil
	}

	// 如果是目录，则递归查找
	var files []string
	var compiledRegex *regexp.Regexp
	if regex != "" {
		compiledRegex = regexp.MustCompile(regex)
	}

	err = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// 跳过隐藏文件和目录 (以点开头)
		base := filepath.Base(p)
		if strings.HasPrefix(base, ".") && base != "." {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !info.IsDir() {
			// 过滤文件
			if compiledRegex == nil || compiledRegex.MatchString(base) {
				files = append(files, p)
			}
		}
		return nil
	})

	return files, err
}
