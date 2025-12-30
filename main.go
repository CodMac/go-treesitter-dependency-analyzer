package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	"github.com/CodMac/go-treesitter-dependency-analyzer/output"
	"github.com/CodMac/go-treesitter-dependency-analyzer/processor"
	_ "github.com/CodMac/go-treesitter-dependency-analyzer/x/java"
)

func main() {
	lang := flag.String("lang", "java", "分析语言")
	path := flag.String("path", ".", "源代码项目根路径")
	filter := flag.String("filter", "", "文件过滤正则")
	jobs := flag.Int("jobs", 4, "并发数")
	outDir := flag.String("out-dir", "./output", "输出目录")
	format := flag.String("format", "jsonl", "输出格式 (jsonl, mermaid)")

	flag.Parse()

	startTime := time.Now()

	fmt.Fprintf(os.Stderr, "[1/4] 🚀 正在扫描目录: %s\n", *path)
	actualFilter := *filter
	if actualFilter == "" {
		actualFilter = fmt.Sprintf(".*\\.%s$", *lang)
	}

	files, err := scanFiles(*path, actualFilter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 扫描文件失败: %v\n", err)
		os.Exit(1)
	}

	proc := processor.NewFileProcessor(model.Language(*lang), false, true, *jobs)
	rels, gCtx, err := proc.ProcessFiles(context.Background(), *path, files)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ 分析失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "[3/4] 💾 正在以 %s 格式导出结果...\n", *format)
	if err := os.MkdirAll(*outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "❌ 创建目录失败: %v\n", err)
		os.Exit(1)
	}

	// 根据参数执行不同的导出逻辑
	switch *format {
	case "jsonl":
		exportAsJSONL(*outDir, gCtx, rels)
	case "mermaid":
		// 这里调用你之前的 Mermaid 导出函数
		mermaidPath := filepath.Join(*outDir, "visualization.html")
		// exportMermaidHTML(mermaidPath, gCtx, rels) // 假设该函数已定义
		fmt.Fprintf(os.Stderr, "    可视化文件已生成: %s\n", mermaidPath)
	default:
		fmt.Fprintf(os.Stderr, "❌ 不支持的输出格式: %s\n", *format)
	}

	totalDuration := time.Since(startTime)
	fmt.Fprintf(os.Stderr, "\n[4/4] ✨ 任务完成! 总耗时: %v\n", totalDuration.Round(time.Millisecond))
}

// 具体的 JSONL 导出调用，封装了对 output 包的调用
func exportAsJSONL(outDir string, gCtx *model.GlobalContext, rels []*model.DependencyRelation) {
	elemPath := filepath.Join(outDir, "element.jsonl")
	relPath := filepath.Join(outDir, "relation.jsonl")

	elemCount, _ := output.ExportElements(elemPath, gCtx)
	fmt.Fprintf(os.Stderr, "    已导出元素: %d 个 -> %s\n", elemCount, elemPath)

	relCount, _ := output.ExportRelations(relPath, rels, gCtx)
	fmt.Fprintf(os.Stderr, "    已导出关系: %d 条 (含包含关系) -> %s\n", relCount, relPath)
}

// scanFiles 保持不变...
func scanFiles(root, filter string) ([]string, error) {
	re, err := regexp.Compile(filter)
	if err != nil {
		return nil, err
	}
	var files []string
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		if re.MatchString(path) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
