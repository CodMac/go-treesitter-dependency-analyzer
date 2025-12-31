package output

import (
	"encoding/json"
	"io"
	"os"

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
	for _, entries := range gCtx.DefinitionsByQN {
		for _, entry := range entries {
			if err := writer.Write(entry.Element); err != nil {
				return count, err
			}
			count++
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
	for _, rel := range rels {
		if err := writer.Write(rel); err != nil {
			return count, err
		}
		count++
	}

	return count, nil
}
