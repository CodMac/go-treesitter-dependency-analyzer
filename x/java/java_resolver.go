package java

import (
	"strings"

	"github.com/CodMac/go-treesitter-dependency-analyzer/context"
	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
)

type SymbolResolver struct{}

func NewJavaSymbolResolver() *SymbolResolver {
	return &SymbolResolver{}
}

func (j *SymbolResolver) BuildQualifiedName(parentQN, name string) string {
	if parentQN == "" || parentQN == "." {
		return name
	}
	return parentQN + "." + name
}

func (j *SymbolResolver) RegisterPackage(gc *context.GlobalContext, packageName string) {
	parts := strings.Split(packageName, ".")
	var current []string
	for _, part := range parts {
		current = append(current, part)
		pkgQN := strings.Join(current, ".")
		if _, ok := gc.DefinitionsByQN[pkgQN]; !ok {
			gc.DefinitionsByQN[pkgQN] = []*context.DefinitionEntry{{
				Element: &model.CodeElement{Kind: model.Package, Name: part, QualifiedName: pkgQN},
			}}
		}
	}
}

func (j *SymbolResolver) Resolve(gc *context.GlobalContext, fc *context.FileContext, symbol string) []*context.DefinitionEntry {
	gc.RLock()
	defer gc.RUnlock()

	// 1. 局部定义
	if defs, ok := fc.DefinitionsBySN[symbol]; ok {
		return defs
	}

	// 2. 精确导入
	if imp, ok := fc.Imports[symbol]; ok {
		if defs, found := gc.DefinitionsByQN[imp.RawImportPath]; found {
			return defs
		}
	}

	// 3. 同包前缀
	pkgQN := j.BuildQualifiedName(fc.PackageName, symbol)
	if defs, ok := gc.DefinitionsByQN[pkgQN]; ok {
		return defs
	}

	// 4. Java 特有的通配符导入
	for _, imp := range fc.Imports {
		if imp.IsWildcard {
			basePath := strings.TrimSuffix(imp.RawImportPath, "*")
			if defs, ok := gc.DefinitionsByQN[basePath+symbol]; ok {
				return defs
			}
		}
	}

	// 5. 兜底：直接按 QN 查找 (处理代码中使用全限定名调用的情况)
	if defs, ok := gc.DefinitionsByQN[symbol]; ok {
		return defs
	}

	return nil
}
