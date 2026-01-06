package noisefilter

import "github.com/CodMac/go-treesitter-dependency-analyzer/model"

// NoiseFilter 定义了如何识别特定语言中的背景噪音
type NoiseFilter interface {
	IsNoise(qualifiedName string) bool
}

var noiseFilterMap = make(map[model.Language]NoiseFilter)

// RegisterNoiseFilter 注册一个语言与其对应的 NoiseFilter
func RegisterNoiseFilter(lang model.Language, noiseFilter NoiseFilter) {
	noiseFilterMap[lang] = noiseFilter
}

// GetNoiseFilter 根据语言类型获取对应的 NoiseFilter 实例。
func GetNoiseFilter(lang model.Language) (NoiseFilter, error) {
	noiseFilter, ok := noiseFilterMap[lang]
	if !ok {
		// 如果没注册，返回一个默认不进行过滤的过滤器，防止程序奔溃
		return &DefaultNoiseFilter{}, nil
	}

	return noiseFilter, nil
}

// DefaultNoiseFilter 默认过滤器：不对任何 QN 进行噪音判定
type DefaultNoiseFilter struct{}

func (d *DefaultNoiseFilter) IsNoise(qn string) bool { return false }
