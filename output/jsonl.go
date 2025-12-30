package output

import (
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
)

type JSONLWriter struct {
	encoder *json.Encoder
}

func NewJSONLWriter(w io.Writer) *JSONLWriter {
	return &JSONLWriter{
		encoder: json.NewEncoder(w),
	}
}

func (w *JSONLWriter) Write(v interface{}) error {
	return w.encoder.Encode(v)
}

// ExportElements 封装了导出 Element 的核心逻辑，包括 PACKAGE 节点的动态生成
func ExportElements(path string, gCtx *model.GlobalContext) (int, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	writer := NewJSONLWriter(f)
	count := 0

	// 1. 导出已注册的定义 (含 Class, Method, File)
	for _, entries := range gCtx.DefinitionsByQN {
		for _, entry := range entries {
			if err := writer.Write(entry.Element); err != nil {
				return count, err
			}
			count++
		}
	}

	// 2. 动态生成并导出唯一的 PACKAGE 节点
	seenPackages := make(map[string]bool)
	for _, fc := range gCtx.FileContexts {
		if fc.PackageName == "" {
			continue
		}
		parts := strings.Split(fc.PackageName, ".")
		var current []string
		for _, part := range parts {
			current = append(current, part)
			pkgQN := strings.Join(current, ".")
			if !seenPackages[pkgQN] {
				pkgElem := &model.CodeElement{
					Kind:          model.Package,
					Name:          part,
					QualifiedName: pkgQN,
				}
				if err := writer.Write(pkgElem); err != nil {
					return count, err
				}
				seenPackages[pkgQN] = true
				count++
			}
		}
	}
	return count, nil
}

// ExportRelations 封装了导出 Relation 的核心逻辑，包括 CONTAINS 关系的补全
func ExportRelations(path string, rels []*model.DependencyRelation, gCtx *model.GlobalContext) (int, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	writer := NewJSONLWriter(f)
	count := 0

	// 1. 导出原始关系
	for _, rel := range rels {
		if err := writer.Write(rel); err != nil {
			return count, err
		}
		count++
	}

	// 2. 自动补全包含关系 (CONTAINS)
	//for _, fc := range gCtx.FileContexts {
	//	// PACKAGE -> PACKAGE
	//	if fc.PackageName != "" {
	//		rel := &model.DependencyRelation{
	//			Type:   "CONTAINS",
	//			Source: &model.CodeElement{Kind: model.Package, QualifiedName: fc.PackageName},
	//			Target: &model.CodeElement{Kind: model.File, QualifiedName: fc.FilePath},
	//		}
	//		writer.Write(rel)
	//		count++
	//	}
	//
	//	// FILE -> TOP_LEVEL_CLASSES
	//	for _, entries := range fc.DefinitionsBySN {
	//		for _, entry := range entries {
	//			if entry.ParentQN == "" || entry.ParentQN == fc.PackageName {
	//				rel := &model.DependencyRelation{
	//					Type:   "CONTAINS",
	//					Source: &model.CodeElement{Kind: model.File, QualifiedName: fc.FilePath},
	//					Target: entry.Element,
	//				}
	//				writer.Write(rel)
	//				count++
	//			}
	//		}
	//	}
	//}

	return count, nil
}
