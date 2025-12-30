package output

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
)

// ExportMermaidHTML 生成包含 Mermaid.js 渲染逻辑的静态网页
func ExportMermaidHTML(outputPath string, gCtx *model.GlobalContext, rels []*model.DependencyRelation) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// 1. 写入 HTML 模板头部
	f.WriteString(`<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Codebase Dependency Map</title>
    <script src="https://cdn.jsdelivr.net/npm/mermaid/dist/mermaid.min.js"></script>
    <style>
        body { font-family: -apple-system, sans-serif; background: #f0f2f5; margin: 20px; }
        .mermaid { background: white; padding: 20px; border-radius: 12px; box-shadow: 0 4px 15px rgba(0,0,0,0.1); }
        h1 { color: #1a1a1a; text-align: center; }
    </style>
</head>
<body>
    <h1>Architecture Visualization</h1>
    <div class="mermaid">
    graph LR
`)

	// 2. 生成层级 Subgraphs
	// 按 Package 分组
	packageGroups := make(map[string][]*model.FileContext)
	for _, fc := range gCtx.FileContexts {
		packageGroups[fc.PackageName] = append(packageGroups[fc.PackageName], fc)
	}

	for pkgName, fcs := range packageGroups {
		hasPkg := pkgName != ""
		if hasPkg {
			fmt.Fprintf(f, "    subgraph \"📦 %s\"\n", pkgName)
		}

		for _, fc := range fcs {
			// 文件作为更细一级的 subgraph
			fmt.Fprintf(f, "        subgraph \"📄 %s\"\n", filepath.Base(fc.FilePath))
			for _, entries := range fc.DefinitionsBySN {
				for _, entry := range entries {
					// 节点：ID["Name (Kind)"]
					id := safeID(entry.Element.QualifiedName)
					fmt.Fprintf(f, "            %s[\"%s <small>(%s)</small>\"]\n", id, entry.Element.Name, entry.Element.Kind)
				}
			}
			f.WriteString("        end\n")
		}

		if hasPkg {
			f.WriteString("    end\n")
		}
	}

	// 3. 生成逻辑依赖关系
	for _, rel := range rels {
		arrow := "-->"
		// 根据类型定制箭头样式
		switch rel.Type {
		case "INHERIT", "IMPLEMENT":
			arrow = "==继承/实现==>"
		case "IMPORT":
			arrow = "-.导入.->"
		}

		fmt.Fprintf(f, "    %s %s %s\n",
			safeID(rel.Source.QualifiedName),
			arrow,
			safeID(rel.Target.QualifiedName))
		// 过滤掉包含关系，Mermaid 通过 subgraph 已经体现了层级
		if rel.Type != "CONTAINS" {

		}
	}

	// 4. 写入脚本初始化和结尾
	f.WriteString(`    </div>
    <script>
        mermaid.initialize({ 
            startOnLoad: true, 
            maxTextSize: 100000,
            theme: 'default',
            flowchart: { useMaxWidth: false, htmlLabels: true }
        });
    </script>
</body>
</html>`)

	return nil
}

// safeID 确保 QualifiedName 符合 Mermaid 的 ID 命名规范
func safeID(id string) string {
	r := strings.NewReplacer(".", "_", "/", "_", "-", "_", "\\", "_", ":", "_", "@", "_")
	return "n_" + r.Replace(id)
}
