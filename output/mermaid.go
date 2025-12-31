package output

import (
	"fmt"
	"os"
	"strings"

	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
)

// safeID 清洗字符串，使其符合 Mermaid 的 ID 命名规范
func safeID(id string) string {
	r := strings.NewReplacer(
		".", "_",
		"(", "_",
		")", "_",
		"[", "_",
		"]", "_",
		" ", "_",
		"-", "_",
		"*", "all",
		"/", "_",
		"\\", "_",
	)
	return "n_" + r.Replace(id)
}

// isFineGrained 判断是否为细粒度节点（方法、字段等）
func isFineGrained(kind model.ElementKind) bool {
	return kind == model.Method || kind == model.Field || kind == model.Variable || kind == model.EnumConstant
}

func ExportMermaidHTML(outputPath string, gCtx *model.GlobalContext, rels []*model.DependencyRelation) error {
	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// 写入 HTML 头部和样式
	fmt.Fprintln(f, `<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <title>Codebase Architecture Map</title>
  <script src="https://cdn.jsdelivr.net/npm/mermaid/dist/mermaid.min.js"></script>
  <style>
    body { font-family: sans-serif; background: #f4f7f6; padding: 20px; }
    .mermaid { background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
  </style>
</head>
<body>
  <h1>Architecture Visualization</h1>
  <div class="mermaid">
  graph LR`)

	// 定义 Mermaid 节点样式
	fmt.Fprintln(f, "  classDef pkg fill:#fff4dd,stroke:#d4a017,stroke-width:2px;")
	fmt.Fprintln(f, "  classDef file fill:#e1f5fe,stroke:#01579b,stroke-width:1px;")
	fmt.Fprintln(f, "  classDef clazz fill:#fff,stroke:#333,stroke-width:1px;")

	// 1. 先声明所有的节点和层级 (过滤掉方法级)
	// 我们遍历 GlobalContext 来构建结构，而不是依赖 rels
	gCtx.RLock()
	for _, fCtx := range gCtx.FileContexts {
		fmt.Fprintf(f, "  subgraph %s [📄 %s]\n", safeID(fCtx.FilePath), fCtx.FilePath)
		for _, entries := range fCtx.DefinitionsBySN {
			for _, entry := range entries {
				// 💡 过滤：只展示类、接口、枚举级别
				if isFineGrained(entry.Element.Kind) {
					continue
				}
				nodeID := safeID(entry.Element.QualifiedName)
				label := fmt.Sprintf("%s <small>(%s)</small>", entry.Element.Name, entry.Element.Kind)
				fmt.Fprintf(f, "    %s[\"%s\"]\n", nodeID, label)
				fmt.Fprintf(f, "    class %s clazz\n", nodeID)
			}
		}
		fmt.Fprintln(f, "  end")
		fmt.Fprintf(f, "  class %s file\n", safeID(fCtx.FilePath))
	}
	gCtx.RUnlock()

	// 2. 导出逻辑关系 (过滤掉方法级依赖)
	for _, rel := range rels {
		// 跳过层级包含关系（已经通过 subgraph 展示了）
		if rel.Type == "CONTAINS" {
			continue
		}

		// 💡 过滤：如果 Source 或 Target 是方法/变量，则不显示这条线
		if isFineGrained(rel.Source.Kind) || isFineGrained(rel.Target.Kind) {
			continue
		}

		srcID := safeID(rel.Source.QualifiedName)
		tgtID := safeID(rel.Target.QualifiedName)

		// 避免指向自身的连线
		if srcID == tgtID {
			continue
		}

		arrow := "-->"
		if rel.Type == model.Import {
			arrow = "-.导入.->"
		} else if rel.Type == model.Extend || rel.Type == model.Implement {
			arrow = "==继承/实现==>"
		}

		fmt.Fprintf(f, "  %s %s %s\n", srcID, arrow, tgtID)
	}

	fmt.Fprintln(f, `  </div>
  <script>
    mermaid.initialize({ startOnLoad: true, maxTextSize: 90000, securityLevel: 'loose' });
  </script>
</body>
</html>`)

	return nil
}
