package java

import "strings"

type NoiseFilter struct{}

func NewJavaNoiseFilter() *NoiseFilter {
	return &NoiseFilter{}
}

func (f *NoiseFilter) IsNoise(qn string) bool {
	noisePrefixes := []string{
		"java.", "javax.", "sun.", "com.sun.", "lombok.",
		"org.slf4j.", "org.apache.log4j.", "bool", "int",
		"long", "float", "double", "char", "byte", "void", "String",
	}
	for _, p := range noisePrefixes {
		if strings.HasPrefix(qn, p) {
			return true
		}
	}
	return false
}
