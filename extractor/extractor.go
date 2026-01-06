package extractor

import (
	"fmt"

	"github.com/CodMac/go-treesitter-dependency-analyzer/context"
	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
)

// Extractor 用于提取关系，需要全局上下文。
type Extractor interface {
	// Extract 基于全局上下文，返回文件中的依赖关系。
	Extract(filePath string, gCtx *context.GlobalContext) ([]*model.DependencyRelation, error)
}

var extractorMap = make(map[model.Language]Extractor)

// RegisterExtractor 注册一个语言与其对应的 Extractor
func RegisterExtractor(lang model.Language, extractor Extractor) {
	extractorMap[lang] = extractor
}

// GetExtractor 根据语言类型获取对应的 Extractor 实例。
func GetExtractor(lang model.Language) (Extractor, error) {
	extractor, ok := extractorMap[lang]
	if !ok {
		return nil, fmt.Errorf("no extractor registered for language: %s", lang)
	}

	return extractor, nil
}
