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
	return &Collector{resolver: resolver}
}

// --- 核心流程 ---

func (c *Collector) CollectDefinitions(rootNode *sitter.Node, filePath string, sourceBytes *[]byte) (*core.FileContext, error) {
	fCtx := core.NewFileContext(filePath, rootNode, sourceBytes)

	// 1. 预处理：包名与导入
	c.processTopLevelDeclarations(fCtx)

	// 2. 阶段一：基础定义收集与唯一 QN 分配
	nameOccurrence := make(map[string]int)
	c.collectBasicDefinitions(fCtx.RootNode, fCtx, fCtx.PackageName, nameOccurrence)

	// 3. 阶段二：填充详细元数据
	c.enrichMetadata(fCtx)

	return fCtx, nil
}

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
		if elem, kind := c.identifyElement(node, fCtx, currentQN); elem != nil {
			c.applyUniqueQN(elem, node, currentQN, occurrences, fCtx.SourceBytes)
			fCtx.AddDefinition(elem, currentQN, node)

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
	var kind model.ElementKind
	var name string

	kindStr := node.Kind()
	switch kindStr {
	case "class_declaration":
		kind = model.Class
	case "interface_declaration":
		kind = model.Interface
	case "enum_declaration":
		kind = model.Enum
	case "method_declaration", "constructor_declaration":
		kind = model.Method
	case "field_declaration", "local_variable_declaration":
		kind = c.determineVariableKind(kindStr)
		name = c.extractVariableName(node, fCtx.SourceBytes)
	case "formal_parameter", "spread_parameter":
		kind = model.Variable
		name = c.extractVariableName(node, fCtx.SourceBytes)
	case "lambda_expression":
		kind, name = model.Lambda, "lambda"
	case "static_initializer":
		kind = model.ScopeBlock
		name = "$static"
	case "block":
		parent := node.Parent()
		if parent == nil {
			return nil, ""
		}
		pKind := parent.Kind()
		if pKind == "class_body" {
			kind = model.ScopeBlock
			name = "$instance"
		} else if pKind == "method_declaration" || pKind == "constructor_declaration" || pKind == "static_initializer" {
			// 透明化处理
			return nil, ""
		} else {
			kind = model.ScopeBlock
			name = "block"
		}
	case "object_creation_expression":
		if c.findNamedChildOfType(node, "class_body") != nil {
			kind, name = model.AnonymousClass, "anonymousClass"
		}
	}

	if kind != "" && name == "" {
		name = c.resolveMissingName(node, kind, parentQN, fCtx.SourceBytes)
	}

	if kind == "" || name == "" {
		return nil, ""
	}

	return &model.CodeElement{
		Kind:     kind,
		Name:     name,
		Path:     fCtx.FilePath,
		Location: c.extractLocation(node, fCtx.FilePath),
	}, kind
}

// --- 第二阶段：元数据富化 ---

func (c *Collector) enrichMetadata(fCtx *core.FileContext) {
	for _, entries := range fCtx.DefinitionsBySN {
		for _, entry := range entries {
			c.processMetadataForEntry(entry, fCtx)
		}
	}
}

func (c *Collector) processMetadataForEntry(entry *core.DefinitionEntry, fCtx *core.FileContext) {
	node, elem := entry.Node, entry.Element
	mods, annos := c.extractModifiersAndAnnotations(node, *fCtx.SourceBytes)
	elem.Doc, elem.Comment = c.extractComments(node, fCtx.SourceBytes)

	extra := &model.Extra{
		Modifiers:   mods,
		Annotations: annos,
		Mores:       make(map[string]interface{}),
	}

	isStatic, isFinal := c.contains(mods, "static"), c.contains(mods, "final")

	switch elem.Kind {
	case model.Method:
		c.fillMethodMetadata(elem, node, extra, mods, fCtx)
	case model.Class, model.Interface:
		c.fillTypeMetadata(elem, node, extra, mods, isFinal, fCtx)
	case model.Field, model.Variable:
		c.fillVariableMetadata(elem, node, extra, mods, isStatic, isFinal, fCtx)
	case model.EnumConstant:
		if args := node.ChildByFieldName("arguments"); args != nil {
			extra.Mores[EnumArguments] = c.getNodeContent(args, *fCtx.SourceBytes)
		}
	case model.ScopeBlock:
		isStaticBlock := (node.Kind() == "static_initializer")
		extra.Mores[BlockIsStatic] = isStaticBlock
		prefix := ""
		if isStaticBlock {
			prefix = "static "
		}
		elem.Signature = prefix + "{...}"
	}
	elem.Extra = extra
}

// --- 专项提取逻辑 ---

func (c *Collector) fillMethodMetadata(elem *model.CodeElement, node *sitter.Node, extra *model.Extra, mods []string, fCtx *core.FileContext) {
	extra.Mores[MethodIsConstructor] = (node.Kind() == "constructor_declaration")
	retType := ""
	if tNode := node.ChildByFieldName("type"); tNode != nil {
		retType = c.getNodeContent(tNode, *fCtx.SourceBytes)
		extra.Mores[MethodReturnType] = retType
	}
	if params := c.extractParameterList(node, fCtx.SourceBytes); len(params) > 0 {
		extra.Mores[MethodParameters] = params
	}
	if throws := c.extractThrows(node, fCtx.SourceBytes); len(throws) > 0 {
		extra.Mores[MethodThrowsTypes] = throws
	}
	paramsRaw := c.extractParameterWithNames(node, fCtx.SourceBytes)
	elem.Signature = strings.TrimSpace(fmt.Sprintf("%s %s %s%s", strings.Join(mods, " "), retType, elem.Name, paramsRaw))
}

func (c *Collector) fillVariableMetadata(elem *model.CodeElement, node *sitter.Node, extra *model.Extra, mods []string, isStatic, isFinal bool, fCtx *core.FileContext) {
	vType := c.extractTypeString(node, fCtx.SourceBytes)
	extra.Mores[VariableType] = vType
	extra.Mores[VariableIsFinal] = isFinal

	if elem.Kind == model.Field {
		extra.Mores[FieldIsStatic] = isStatic
		extra.Mores[FieldIsFinal] = isFinal
		extra.Mores[FieldIsConstant] = isStatic && isFinal
		extra.Mores[FieldType] = vType
	} else {
		nodeKind := node.Kind()
		extra.Mores[VariableIsParam] = (nodeKind == "formal_parameter" || nodeKind == "spread_parameter")
	}
	elem.Signature = strings.TrimSpace(fmt.Sprintf("%s %s %s", strings.Join(mods, " "), vType, elem.Name))
}

func (c *Collector) fillTypeMetadata(elem *model.CodeElement, node *sitter.Node, extra *model.Extra, mods []string, isFinal bool, fCtx *core.FileContext) {
	extra.Mores[ClassIsAbstract] = c.contains(mods, "abstract")
	extra.Mores[ClassIsFinal] = isFinal

	heritage := ""
	if super := node.ChildByFieldName("superclass"); super != nil {
		content := c.getNodeContent(super, *fCtx.SourceBytes)
		extra.Mores[ClassSuperClass] = strings.TrimPrefix(content, "extends ")
		heritage += " " + content
	}

	var ifacesNode *sitter.Node
	if n := node.ChildByFieldName("interfaces"); n != nil {
		ifacesNode = n
	} else if n := node.ChildByFieldName("extends_interfaces"); n != nil {
		ifacesNode = n
	} else if n := c.findNamedChildOfType(node, "interfaces"); n != nil {
		ifacesNode = n
	} else if n := c.findNamedChildOfType(node, "extends_interfaces"); n != nil {
		ifacesNode = n
	}

	if ifacesNode != nil {
		ifaces := c.extractInterfaceListFromNode(ifacesNode, fCtx.SourceBytes)
		if len(ifaces) > 0 {
			mKey := InterfaceImplementedInterfaces
			if elem.Kind == model.Class {
				mKey = ClassImplementedInterfaces
			}
			extra.Mores[mKey] = ifaces
			heritage += " " + c.getNodeContent(ifacesNode, *fCtx.SourceBytes)
		}
	}

	typeParams := ""
	if tpNode := node.ChildByFieldName("type_parameters"); tpNode != nil {
		typeParams = c.getNodeContent(tpNode, *fCtx.SourceBytes)
	}

	displayKind := strings.Replace(node.Kind(), "_declaration", "", 1)
	elem.Signature = strings.TrimSpace(fmt.Sprintf("%s %s %s%s%s", strings.Join(mods, " "), displayKind, elem.Name, typeParams, heritage))
}

// --- 提取相关函数 ---

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

func (c *Collector) extractTypeString(node *sitter.Node, src *[]byte) string {
	// 针对 formal_parameter 尝试通过 field name 寻找
	if tNode := node.ChildByFieldName("type"); tNode != nil {
		return c.getNodeContent(tNode, *src)
	}

	// 针对 spread_parameter (Object... values) 的特殊处理
	// 在 Java AST 中，spread_parameter 的结构通常包含一个 type 节点和 ... 符号
	if node.Kind() == "spread_parameter" {
		for i := 0; i < int(node.NamedChildCount()); i++ {
			child := node.NamedChild(uint(i))
			if strings.Contains(child.Kind(), "type") {
				return c.getNodeContent(child, *src) + "..."
			}
		}
	}
	return "unknown"
}

func (c *Collector) extractParameterTypesOnly(node *sitter.Node, src *[]byte) string {
	pNode := node.ChildByFieldName("parameters")
	if pNode == nil {
		return "()"
	}

	var types []string
	for i := 0; i < int(pNode.NamedChildCount()); i++ {
		param := pNode.NamedChild(uint(i))
		tStr := c.extractTypeString(param, src) // 统一调用 extractTypeString

		// 去掉泛型，只保留基础类型名用于 QN 匹配
		tStr = strings.Split(tStr, "<")[0]
		types = append(types, strings.TrimSpace(tStr))
	}
	return "(" + strings.Join(types, ",") + ")"
}

func (c *Collector) extractVariableName(node *sitter.Node, src *[]byte) string {
	if nNode := node.ChildByFieldName("name"); nNode != nil {
		return c.getNodeContent(nNode, *src)
	}
	// 处理变数定义中的 declarator
	if vd := c.findNamedChildOfType(node, "variable_declarator"); vd != nil {
		if nNode := vd.ChildByFieldName("name"); nNode != nil {
			return c.getNodeContent(nNode, *src)
		}
	}
	return ""
}

func (c *Collector) extractModifiersAndAnnotations(n *sitter.Node, src []byte) ([]string, []string) {
	var mods, annos []string
	mNode := n.ChildByFieldName("modifiers")
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

func (c *Collector) extractComments(node *sitter.Node, src *[]byte) (doc string, comment string) {
	curr := node
	if node.Kind() == "variable_declarator" && node.Parent() != nil {
		curr = node.Parent()
	}
	prev := curr.PrevSibling()
	for prev != nil {
		if prev.Kind() == "block_comment" || prev.Kind() == "line_comment" {
			text := c.getNodeContent(prev, *src)
			if strings.HasPrefix(text, "/**") {
				doc = text
			} else {
				comment = text
			}
			break
		}
		if strings.TrimSpace(c.getNodeContent(prev, *src)) != "" {
			break
		}
		prev = prev.PrevSibling()
	}
	return
}

func (c *Collector) extractInterfaceListFromNode(node *sitter.Node, src *[]byte) []string {
	var results []string
	target := node
	if listNode := c.findNamedChildOfType(node, "type_list"); listNode != nil {
		target = listNode
	}
	for i := 0; i < int(target.NamedChildCount()); i++ {
		child := target.NamedChild(uint(i))
		if strings.Contains(child.Kind(), "type") {
			results = append(results, c.getNodeContent(child, *src))
		}
	}
	return results
}

func (c *Collector) extractParameterList(node *sitter.Node, src *[]byte) []string {
	pNode := node.ChildByFieldName("parameters")
	if pNode == nil {
		return nil
	}
	var params []string
	for i := 0; i < int(pNode.NamedChildCount()); i++ {
		params = append(params, c.getNodeContent(pNode.NamedChild(uint(i)), *src))
	}
	return params
}

func (c *Collector) extractParameterWithNames(node *sitter.Node, src *[]byte) string {
	pNode := node.ChildByFieldName("parameters")
	if pNode == nil {
		return "()"
	}
	return c.getNodeContent(pNode, *src)
}

func (c *Collector) extractThrows(node *sitter.Node, src *[]byte) []string {
	tNode := node.ChildByFieldName("throws")
	if tNode == nil {
		tNode = c.findNamedChildOfType(node, "throws")
	}
	if tNode == nil {
		return nil
	}
	var types []string
	for i := 0; i < int(tNode.NamedChildCount()); i++ {
		types = append(types, c.getNodeContent(tNode.NamedChild(uint(i)), *src))
	}
	return types
}

func (c *Collector) applyUniqueQN(elem *model.CodeElement, node *sitter.Node, parentQN string, occurrences map[string]int, src *[]byte) {
	identity := elem.Name
	if elem.Kind == model.Method {
		identity += c.extractParameterTypesOnly(node, src)
	}
	occurrences[identity]++
	count := occurrences[identity]

	if elem.Kind == model.AnonymousClass || elem.Kind == model.Lambda || elem.Kind == model.ScopeBlock || count > 1 {
		identity = fmt.Sprintf("%s$%d", identity, count)
	}
	elem.QualifiedName = c.resolver.BuildQualifiedName(parentQN, identity)
}

func (c *Collector) resolveMissingName(node *sitter.Node, kind model.ElementKind, parentQN string, src *[]byte) string {
	if nNode := node.ChildByFieldName("name"); nNode != nil {
		return c.getNodeContent(nNode, *src)
	}
	if kind == model.Method {
		parts := strings.Split(parentQN, ".")
		return parts[len(parts)-1]
	}
	return ""
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

func (c *Collector) getNodeContent(n *sitter.Node, src []byte) string {
	if n == nil {
		return ""
	}
	return n.Utf8Text(src)
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
		k == model.Method || k == model.Lambda || k == model.ScopeBlock ||
		k == model.AnonymousClass
}

func (c *Collector) contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (c *Collector) determineVariableKind(kindStr string) model.ElementKind {
	if kindStr == "local_variable_declaration" {
		return model.Variable
	}
	return model.Field
}
