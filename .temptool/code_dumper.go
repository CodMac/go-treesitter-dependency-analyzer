package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var ignoreList = map[string]bool{
	".git": true, ".idea": true, ".vscode": true, "node_modules": true,
	"vendor": true, "bin": true, "obj": true,
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("用法: go run main.go <目标文件夹> [后缀:.go,.java] [输出文件路径:result.txt]")
		return
	}

	targetDir := os.Args[1]
	filterStr := ""
	if len(os.Args) > 2 {
		filterStr = os.Args[2]
	}

	// 1. 设置输出目标
	var out io.Writer = os.Stdout
	saveToFile := false
	if len(os.Args) > 3 && os.Args[3] != "" {
		outPath := os.Args[3]
		file, err := os.Create(outPath)
		if err != nil {
			fmt.Printf("创建输出文件失败: %v\n", err)
			return
		}
		defer file.Close()
		out = file
		saveToFile = true
	}

	// 2. 解析后缀
	var filterExts []string
	if filterStr != "" && filterStr != "all" && filterStr != "\"\"" {
		for _, p := range strings.Split(filterStr, ",") {
			ext := strings.TrimSpace(p)
			if !strings.HasPrefix(ext, ".") {
				ext = "." + ext
			}
			filterExts = append(filterExts, strings.ToLower(ext))
		}
	}

	shouldInclude := func(filename string) bool {
		if len(filterExts) == 0 {
			return true
		}
		ext := strings.ToLower(filepath.Ext(filename))
		for _, f := range filterExts {
			if ext == f {
				return true
			}
		}
		return false
	}

	// 开始写入内容
	fmt.Fprintln(out, "================================================================================")
	fmt.Fprintf(out, "📂 项目目录结构: %s\n", targetDir)
	if len(filterExts) > 0 {
		fmt.Fprintf(out, "🔍 已启用后缀过滤: %v\n", filterExts)
	}
	fmt.Fprintln(out, "================================================================================")

	// 打印目录树
	filepath.WalkDir(targetDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || (d.IsDir() && ignoreList[d.Name()]) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() && !shouldInclude(d.Name()) {
			return nil
		}
		relPath, _ := filepath.Rel(targetDir, path)
		if relPath == "." {
			return nil
		}
		indent := strings.Repeat("  ", strings.Count(relPath, string(os.PathSeparator)))
		icon := "📄"
		if d.IsDir() {
			icon = "📁"
		}
		fmt.Fprintf(out, "%s%s %s\n", indent, icon, d.Name())
		return nil
	})

	fmt.Fprintln(out, "\n================================================================================")
	fmt.Fprintln(out, "📄 文件源码内容")
	fmt.Fprintln(out, "================================================================================")

	// 打印文件内容
	filepath.WalkDir(targetDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && ignoreList[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !shouldInclude(d.Name()) {
			return nil
		}

		content, _ := os.ReadFile(path)
		relPath, _ := filepath.Rel(targetDir, path)
		fmt.Fprintf(out, "\n--------------------------------------------------------------------------------\n")
		fmt.Fprintf(out, "FILE: %s\n", relPath)
		fmt.Fprintf(out, "--------------------------------------------------------------------------------\n")
		fmt.Fprintln(out, string(content))
		return nil
	})

	if saveToFile {
		fmt.Printf("✅ 任务完成！结果已保存至: %s\n", os.Args[3])
	}
}
