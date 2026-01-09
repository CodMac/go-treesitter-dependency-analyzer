package java

import (
	"fmt"
	"strings"

	"github.com/CodMac/go-treesitter-dependency-analyzer/core"
	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type Collector struct {
	resolver core.SymbolResolver
}

func NewJavaCollector() *Collector {
	resolver, err := core.GetSymbolResolver(core.LangJava)
	if err != nil {
		panic(err)
	}

	return &Collector{
		resolver: resolver,
	}
}

func (c *Collector) CollectDefinitions(rootNode *sitter.Node, filePath string, sourceBytes *[]byte) (*core.FileContext, error) {
	fCtx := core.NewFileContext(filePath, rootNode, sourceBytes)

	// 1. 扫描顶层节点以确定 Package 和 Imports
	c.processTopLevelDeclarations(fCtx)

	// 2. 第一阶段：递归收集所有基础定义并生成唯一 QN
	nameOccurrence := make(map[string]int)
	initialQN := fCtx.PackageName
	c.collectBasicDefinitions(fCtx.RootNode, fCtx, initialQN, nameOccurrence)

	// 3. 第二阶段：填充元数据 (Mores, Signature, Doc, Comment)
	c.enrichMetadata(fCtx)

	return fCtx, nil
}

// --- 阶段 1: 基础定义扫描 ---

func (c *Collector) processTopLevelDeclarations(fCtx *core.FileContext) {
	for i := 0; i < int(fCtx.RootNode.ChildCount()); i++ {
		child := fCtx.RootNode.Child(uint(i))
		if child == nil {
			continue
		}

		switch child.Kind() {
		case "package_declaration":
			if ident := c.findNamedChildOfType(child, "scoped_identifier"); ident != nil {
				fCtx.PackageName = c.getNodeContent(ident, *fCtx.SourceBytes)
			} else if ident := child.ChildByFieldName("name"); ident != nil {
				fCtx.PackageName = c.getNodeContent(ident, *fCtx.SourceBytes)
			}
		case "import_declaration":
			c.handleImport(child, fCtx)
		}
	}
}

func (c *Collector) collectBasicDefinitions(node *sitter.Node, fCtx *core.FileContext, currentQN string, occurrences map[string]int) {
	if node.IsNamed() {
		elem, kind := c.identifyElement(node, fCtx, currentQN)
		if elem != nil {
			// 生成唯一 QN
			c.applyUniqueQN(elem, node, currentQN, occurrences, fCtx.SourceBytes)

			// 注册定义 (已包含 node)
			fCtx.AddDefinition(elem, currentQN, node)

			// 容器类型进入新作用域
			if c.isScopeContainer(kind) {
				childOccurrences := make(map[string]int)
				for i := 0; i < int(node.ChildCount()); i++ {
					c.collectBasicDefinitions(node.Child(uint(i)), fCtx, elem.QualifiedName, childOccurrences)
				}
				return
			}
		}
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		c.collectBasicDefinitions(node.Child(uint(i)), fCtx, currentQN, occurrences)
	}
}

func (c *Collector) identifyElement(node *sitter.Node, fCtx *core.FileContext, parentQN string) (*model.CodeElement, model.ElementKind) {
	kindStr := node.Kind()
	var kind model.ElementKind
	var name string

	switch kindStr {
	case "class_declaration":
		kind = model.Class
	case "interface_declaration":
		kind = model.Interface
	case "enum_declaration":
		kind = model.Enum
	case "method_declaration", "constructor_declaration":
		kind = model.Method
	case "field_declaration":
		kind = model.Field
	case "variable_declarator":
		kind = model.Variable
	case "formal_parameter":
		kind = model.Variable
	case "lambda_expression":
		kind = model.Lambda
		name = "lambda"
	case "block":
		kind = model.ScopeBlock
		name = "block"
	default:
		return nil, ""
	}

	if name == "" {
		if nNode := node.ChildByFieldName("name"); nNode != nil {
			name = c.getNodeContent(nNode, *fCtx.SourceBytes)
		} else if kind == model.Method { // 构造函数
			parts := strings.Split(parentQN, ".")
			name = parts[len(parts)-1]
		}
	}

	if name == "" {
		return nil, ""
	}

	return &model.CodeElement{
		Kind:     kind,
		Name:     name,
		Path:     fCtx.FilePath,
		Location: c.extractLocation(node, fCtx.FilePath),
	}, kind
}

func (c *Collector) applyUniqueQN(elem *model.CodeElement, node *sitter.Node, parentQN string, occurrences map[string]int, src *[]byte) {
	identity := elem.Name
	if elem.Kind == model.Method {
		identity += c.extractParameterTypesOnly(node, src)
	}

	occurrences[identity]++
	count := occurrences[identity]

	if count > 1 || elem.Kind == model.Lambda || elem.Kind == model.ScopeBlock {
		identity = fmt.Sprintf("%s$%d", identity, count)
	}

	elem.QualifiedName = c.resolver.BuildQualifiedName(parentQN, identity)
}

// --- 阶段 2: 元数据填充 ---

func (c *Collector) enrichMetadata(fCtx *core.FileContext) {
	for _, entries := range fCtx.DefinitionsBySN {
		for _, entry := range entries {
			node := entry.Node
			elem := entry.Element

			// 1. 提取修饰符和注解
			mods, annos := c.extractModifiersAndAnnotations(node, *fCtx.SourceBytes)

			// 2. 提取注释 (Doc/Comment)
			elem.Doc, elem.Comment = c.extractComments(node, fCtx.SourceBytes)

			// 3. 填充 Extra.Mores
			extra := &model.Extra{
				Modifiers:   mods,
				Annotations: annos,
				Mores:       make(map[string]interface{}),
			}

			switch elem.Kind {
			case model.Method:
				extra.Mores[MethodIsConstructor] = node.Kind() == "constructor_declaration"
				retType := ""
				if tNode := node.ChildByFieldName("type"); tNode != nil {
					retType = c.getNodeContent(tNode, *fCtx.SourceBytes)
					extra.Mores[MethodReturnType] = retType
				}
				params := c.extractParameterWithNames(node, fCtx.SourceBytes)
				extra.Mores[MethodFullSignatureQN] = elem.Name + params
				elem.Signature = fmt.Sprintf("%s %s%s", retType, elem.Name, params)

			case model.Class:
				extra.Mores[ClassIsAbstract] = c.contains(mods, "abstract")
				extra.Mores[ClassIsFinal] = c.contains(mods, "final")
				elem.Signature = strings.Join(mods, " ") + " class " + elem.Name
			case model.Interface:
				elem.Signature = strings.Join(mods, " ") + " interface " + elem.Name
			case model.Enum:
				elem.Signature = strings.Join(mods, " ") + " interface " + elem.Name
			case model.Field, model.Variable:
				fType := ""
				if tNode := node.ChildByFieldName("type"); tNode != nil {
					fType = c.getNodeContent(tNode, *fCtx.SourceBytes)
					extra.Mores[FieldType] = fType
				}
				elem.Signature = fType + " " + elem.Name
			}
			elem.Extra = extra
		}
	}
}

// --- 辅助工具函数 ---

func (c *Collector) extractComments(node *sitter.Node, src *[]byte) (doc string, comment string) {
	curr := node
	// 如果是 variable_declarator，注释通常在父节点 field_declaration 上
	if node.Kind() == "variable_declarator" && node.Parent() != nil {
		curr = node.Parent()
	}

	// 尝试寻找紧邻的前一个兄弟节点是否是注释
	prev := curr.PrevSibling()
	for prev != nil {
		if prev.Kind() == "block_comment" || prev.Kind() == "line_comment" {
			text := c.getNodeContent(prev, *src)
			if strings.HasPrefix(text, "/**") {
				doc = text
			} else {
				comment = text
			}
			// 只取最近的一个
			break
		}
		// 如果中间隔了非空白字符，就停止
		if strings.TrimSpace(c.getNodeContent(prev, *src)) != "" {
			break
		}
		prev = prev.PrevSibling()
	}
	return
}

func (c *Collector) handleImport(node *sitter.Node, fCtx *core.FileContext) {
	isStatic := false
	var pathParts []string

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		kind := child.Kind()
		if kind == "static" {
			isStatic = true
			continue
		}
		if kind == "scoped_identifier" || kind == "identifier" || kind == "asterisk" {
			pathParts = append(pathParts, c.getNodeContent(child, *fCtx.SourceBytes))
		}
	}

	if len(pathParts) == 0 {
		return
	}

	fullPath := strings.Join(pathParts, ".")
	isWildcard := strings.HasSuffix(fullPath, ".*") || pathParts[len(pathParts)-1] == "*"

	entry := &core.ImportEntry{
		RawImportPath: fullPath,
		IsWildcard:    isWildcard,
		Location:      c.extractLocation(node, fCtx.FilePath),
	}

	var alias string
	if isWildcard {
		alias = "*"
		entry.Kind = model.Package
	} else {
		parts := strings.Split(fullPath, ".")
		alias = parts[len(parts)-1]
		entry.Kind = model.Class
		if isStatic {
			entry.Kind = model.Constant
		}
	}
	entry.Alias = alias
	fCtx.AddImport(alias, entry)
}

func (c *Collector) extractParameterTypesOnly(node *sitter.Node, src *[]byte) string {
	pNode := node.ChildByFieldName("parameters")
	if pNode == nil {
		return "()"
	}
	var types []string
	for i := 0; i < int(pNode.NamedChildCount()); i++ {
		param := pNode.NamedChild(uint(i))
		tStr := "unknown"
		if tNode := param.ChildByFieldName("type"); tNode != nil {
			tStr = c.getNodeContent(tNode, *src)
		}
		tStr = strings.Split(tStr, "<")[0]
		types = append(types, strings.TrimSpace(tStr))
	}
	return "(" + strings.Join(types, ",") + ")"
}

func (c *Collector) extractParameterWithNames(node *sitter.Node, src *[]byte) string {
	pNode := node.ChildByFieldName("parameters")
	if pNode == nil {
		return "()"
	}
	return c.getNodeContent(pNode, *src)
}

func (c *Collector) extractModifiersAndAnnotations(n *sitter.Node, src []byte) ([]string, []string) {
	var mods, annos []string
	mNode := n.ChildByFieldName("modifiers")
	// 部分 Java 节点 modifiers 可能不是 Field
	if mNode == nil {
		mNode = c.findNamedChildOfType(n, "modifiers")
	}

	if mNode != nil {
		for i := 0; i < int(mNode.ChildCount()); i++ {
			child := mNode.Child(uint(i))
			txt := c.getNodeContent(child, src)
			if strings.Contains(child.Kind(), "annotation") {
				annos = append(annos, txt)
			} else if txt != "" {
				mods = append(mods, txt)
			}
		}
	}
	return mods, annos
}

func (c *Collector) getNodeContent(n *sitter.Node, src []byte) string {
	if n == nil {
		return ""
	}
	return n.Utf8Text(src)
}

func (c *Collector) extractLocation(n *sitter.Node, filePath string) *model.Location {
	return &model.Location{
		FilePath:    filePath,
		StartLine:   int(n.StartPosition().Row) + 1,
		EndLine:     int(n.EndPosition().Row) + 1,
		StartColumn: int(n.StartPosition().Column),
		EndColumn:   int(n.EndPosition().Column),
	}
}

func (c *Collector) findNamedChildOfType(n *sitter.Node, nodeType string) *sitter.Node {
	for i := 0; i < int(n.NamedChildCount()); i++ {
		child := n.NamedChild(uint(i))
		if child.Kind() == nodeType {
			return child
		}
	}
	return nil
}

func (c *Collector) isScopeContainer(k model.ElementKind) bool {
	return k == model.Class || k == model.Interface || k == model.Enum ||
		k == model.Method || k == model.Lambda || k == model.ScopeBlock
}

func (c *Collector) contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
