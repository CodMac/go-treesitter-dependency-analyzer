package output

import (
	"encoding/json"
	"fmt"
	"io"

	"go-treesitter-dependency-analyzer/model"
)

// JSONLWriter 负责将 DependencyRelation 结构体序列化为 JSON Lines 格式并写入输出流。
type JSONLWriter struct {
	Writer io.Writer
}

// NewJSONLWriter 创建一个新的 JSONLWriter 实例。
func NewJSONLWriter(w io.Writer) *JSONLWriter {
	return &JSONLWriter{
		Writer: w,
	}
}

// WriteRelation 将单个 DependencyRelation 结构体转换为 JSON 格式，并在末尾添加换行符，
// 然后写入到 Writer 中。
func (w *JSONLWriter) WriteRelation(relation *model.DependencyRelation) error {
	// 1. 将结构体编码为 JSON 字节
	data, err := json.Marshal(relation)
	if err != nil {
		return fmt.Errorf("failed to marshal dependency relation to JSON: %w", err)
	}

	// 2. 写入 JSON 数据
	_, err = w.Writer.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write JSON data: %w", err)
	}

	// 3. 写入换行符，实现 JSON Lines 格式
	_, err = w.Writer.Write([]byte{'\n'})
	if err != nil {
		return fmt.Errorf("failed to write newline character: %w", err)
	}

	return nil
}

// WriteAllRelations 遍历 DependencyRelation 列表，逐个写入 JSONL 格式。
func (w *JSONLWriter) WriteAllRelations(relations []*model.DependencyRelation) error {
	for _, rel := range relations {
		if err := w.WriteRelation(rel); err != nil {
			// 允许部分写入，但记录错误
			fmt.Fprintf(w.Writer, "Error writing relation: %v\n", err)
			continue
		}
	}
	return nil
}