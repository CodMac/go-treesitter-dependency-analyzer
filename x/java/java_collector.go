package java

import (
	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	sitter "github.com/tree-sitter/go-tree-sitter"

	"strings"
)

// Collector 负责遍历 Java AST 并收集所有符号定义
type Collector struct{}

func NewJavaCollector() *Collector {
	return &Collector{}
}

func (c *Collector) CollectDefinitions(rootNode *sitter.Node, filePath string, sourceBytes *[]byte) (*model.FileContext, error) {
	fCtx := model.NewFileContext(filePath, rootNode, sourceBytes)

	// 1. 扫描顶层节点以确定 Package 和 Imports
	c.processTopLevelDeclarations(fCtx)

	// 2. 启动递归收集
	initialQN := fCtx.PackageName
	if err := c.collectDefinitionsRecursive(fCtx.RootNode, fCtx, initialQN); err != nil {
		return nil, err
	}

	return fCtx, nil
}

func (c *Collector) processTopLevelDeclarations(fCtx *model.FileContext) {
	for i := 0; i < int(fCtx.RootNode.ChildCount()); i++ {
		child := fCtx.RootNode.Child(uint(i))
		if child == nil {
			continue
		}

		switch child.Kind() {
		case "package_declaration":
			for j := 0; j < int(child.ChildCount()); j++ {
				sub := child.Child(uint(j))
				if sub.Kind() == "scoped_identifier" || sub.Kind() == "identifier" {
					fCtx.PackageName = c.getNodeContent(sub, *fCtx.SourceBytes)
					break
				}
			}
		case "import_declaration":
			c.handleImport(child, fCtx)
		}
	}
}

func (c *Collector) handleImport(node *sitter.Node, fCtx *model.FileContext) {
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

	entry := &model.ImportEntry{
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

// --- 核心递归逻辑 ---

func (c *Collector) collectDefinitionsRecursive(node *sitter.Node, fCtx *model.FileContext, currentQNPrefix string) error {
	if node.IsNamed() {
		elem, kind := c.getDefinitionElement(node, fCtx.SourceBytes, fCtx.FilePath, currentQNPrefix)
		if elem != nil {
			parentQN := currentQNPrefix

			// 1. 确定 identityName (处理重载后缀)
			identityName := elem.Name
			if node.Kind() == "compact_constructor_declaration" {
				identityName = elem.Name + c.extractRecordParamsStr(node, fCtx.SourceBytes, false)
			} else if kind == model.Method {
				identityName = elem.Name + c.extractParameterTypesOnly(node, fCtx.SourceBytes)
			}
			elem.QualifiedName = model.BuildQualifiedName(parentQN, identityName)

			// 2. 完善 MethodExtra 信息
			if kind == model.Method && elem.Extra != nil && elem.Extra.MethodExtra != nil {
				paramWithNames := ""
				if node.Kind() == "compact_constructor_declaration" {
					paramWithNames = c.extractRecordParamsStr(node, fCtx.SourceBytes, true)
				} else {
					paramWithNames = c.extractParameterWithNames(node, fCtx.SourceBytes)
				}
				elem.Extra.MethodExtra.IncludeParamNameQN = model.BuildQualifiedName(parentQN, elem.Name+paramWithNames)
			}

			// 3. 注册定义
			fCtx.AddDefinition(elem, parentQN)

			// 4. 处理特殊 Java 特性 (Record)
			if node.Kind() == "formal_parameter" && c.isRecordComponent(node) {
				c.handleRecordComponentAccessor(node, elem, fCtx, parentQN)
			}
			if node.Kind() == "compact_constructor_declaration" {
				c.handleCompactConstructorImplicitParams(node, elem, fCtx)
			}

			// 5. 更新作用域前缀
			if c.isContainerKind(kind) || kind == model.Method {
				currentQNPrefix = elem.QualifiedName
			}
		}
	}

	// 遍历子节点
	cursor := node.Walk()
	defer cursor.Close()
	if cursor.GotoFirstChild() {
		for {
			if err := c.collectDefinitionsRecursive(cursor.Node(), fCtx, currentQNPrefix); err != nil {
				return err
			}
			if !cursor.GotoNextSibling() {
				break
			}
		}
	}
	return nil
}

// --- Record 特性处理插件 ---

func (c *Collector) handleRecordComponentAccessor(node *sitter.Node, fieldElem *model.CodeElement, fCtx *model.FileContext, parentQN string) {
	// 深度克隆原始元素生成 Method 访问器 (e.g., id -> id())
	methodElem := *fieldElem
	newExtra := *fieldElem.Extra
	methodElem.Extra = &newExtra
	methodElem.Kind = model.Method
	methodElem.QualifiedName = model.BuildQualifiedName(parentQN, fieldElem.Name+"()")
	methodElem.Extra.MethodExtra = &model.MethodExtra{
		ReturnType:         fieldElem.Extra.FieldExtra.Type,
		IncludeParamNameQN: methodElem.QualifiedName,
	}
	fCtx.AddDefinition(&methodElem, parentQN)
}

func (c *Collector) handleCompactConstructorImplicitParams(node *sitter.Node, ctorElem *model.CodeElement, fCtx *model.FileContext) {
	// 将 Record 组件注入到紧凑构造函数的作用域
	recordNode := c.findParentRecord(node)
	if recordNode != nil {
		if pNode := recordNode.ChildByFieldName("parameters"); pNode != nil {
			for i := 0; i < int(pNode.NamedChildCount()); i++ {
				paramNode := pNode.NamedChild(uint(i))
				pElem, _ := c.getDefinitionElement(paramNode, fCtx.SourceBytes, fCtx.FilePath, ctorElem.QualifiedName)
				if pElem != nil {
					pElem.Kind = model.Variable
					pElem.QualifiedName = model.BuildQualifiedName(ctorElem.QualifiedName, pElem.Name)
					fCtx.AddDefinition(pElem, ctorElem.QualifiedName)
				}
			}
		}
	}
}

// --- 参数提取逻辑 ---

func (c *Collector) extractParameterTypesOnly(node *sitter.Node, sourceBytes *[]byte) string {
	pNode := node.ChildByFieldName("parameters")
	if pNode == nil {
		if node.Kind() == "annotation_type_element_declaration" {
			return "()"
		}
		return ""
	}
	return c.extractParameterTypesOnlyFromNode(pNode, sourceBytes)
}

func (c *Collector) extractRecordParamsStr(compactCtor *sitter.Node, sourceBytes *[]byte, withNames bool) string {
	record := c.findParentRecord(compactCtor)
	if record == nil {
		return "()"
	}

	pNode := record.ChildByFieldName("parameters")
	if pNode == nil {
		return "()"
	}

	if withNames {
		return c.extractParameterWithNamesFromNode(pNode, sourceBytes)
	}

	return c.extractParameterTypesOnlyFromNode(pNode, sourceBytes)
}

func (c *Collector) extractParameterTypesOnlyFromNode(pNode *sitter.Node, sourceBytes *[]byte) string {
	var types []string
	for i := 0; i < int(pNode.NamedChildCount()); i++ {
		param := pNode.NamedChild(uint(i))
		tStr := c.getTypeString(param, sourceBytes)
		if tStr != "" {
			tStr = strings.Split(tStr, "<")[0] // 泛型擦除
			if (param.Kind() == "spread_parameter" || c.hasEllipsis(param)) && !strings.HasSuffix(tStr, "...") {
				tStr += "..."
			}
			types = append(types, strings.TrimSpace(tStr))
		}
	}
	return "(" + strings.Join(types, ",") + ")"
}

func (c *Collector) extractParameterWithNames(node *sitter.Node, sourceBytes *[]byte) string {
	pNode := node.ChildByFieldName("parameters")
	if pNode == nil {
		if node.Kind() == "annotation_type_element_declaration" {
			return "()"
		}
		return ""
	}
	return c.extractParameterWithNamesFromNode(pNode, sourceBytes)
}

func (c *Collector) extractParameterWithNamesFromNode(pNode *sitter.Node, sourceBytes *[]byte) string {
	var params []string
	for i := 0; i < int(pNode.NamedChildCount()); i++ {
		param := pNode.NamedChild(uint(i))
		tStr, nStr := "unknown", "arg"
		if tNode := param.ChildByFieldName("type"); tNode != nil {
			tStr = c.getNodeContent(tNode, *sourceBytes)
		} else if param.Kind() == "spread_parameter" && param.NamedChildCount() > 0 {
			tStr = c.getNodeContent(param.NamedChild(0), *sourceBytes)
		}
		if nNode := param.ChildByFieldName("name"); nNode != nil {
			nStr = c.getNodeContent(nNode, *sourceBytes)
		} else if param.Kind() == "spread_parameter" {
			nStr = c.getNodeContent(param.NamedChild(param.NamedChildCount()-1), *sourceBytes)
		}

		if (param.Kind() == "spread_parameter" || c.hasEllipsis(param)) && !strings.HasSuffix(tStr, "...") {
			params = append(params, tStr+"... "+nStr)
		} else {
			params = append(params, tStr+" "+nStr)
		}
	}
	return "(" + strings.Join(params, ", ") + ")"
}

// --- 基础提取逻辑 ---

func (c *Collector) getDefinitionElement(node *sitter.Node, sourceBytes *[]byte, filePath string, currentQNPrefix string) (*model.CodeElement, model.ElementKind) {
	elem, kind := c.extractElementBasic(node, sourceBytes, filePath, currentQNPrefix)
	if elem == nil {
		return nil, ""
	}
	c.fillElementExtra(node, elem, kind, sourceBytes)
	return elem, kind
}

func (c *Collector) extractElementBasic(node *sitter.Node, sourceBytes *[]byte, filePath string, currentQNPrefix string) (*model.CodeElement, model.ElementKind) {
	kindStr := node.Kind()
	var kind model.ElementKind
	var nameNode *sitter.Node

	switch kindStr {
	case "class_declaration", "record_declaration":
		kind = model.Class
	case "interface_declaration":
		kind = model.Interface
	case "enum_declaration":
		kind = model.Enum
	case "annotation_type_declaration":
		kind = model.KAnnotation
	case "method_declaration", "constructor_declaration", "compact_constructor_declaration", "annotation_type_element_declaration":
		kind = model.Method
	case "field_declaration":
		kind = model.Field
	case "enum_constant":
		kind = model.EnumConstant
	case "formal_parameter", "spread_parameter":
		if c.isRecordComponent(node) {
			kind = model.Field
		} else {
			kind = model.Variable
		}
	default:
		return nil, ""
	}

	if kindStr == "field_declaration" || kindStr == "spread_parameter" {
		if vNode := c.findNamedChildOfType(node, "variable_declarator"); vNode != nil {
			nameNode = vNode.ChildByFieldName("name")
		}
	} else {
		nameNode = node.ChildByFieldName("name")
	}

	name := ""
	if nameNode != nil {
		name = c.getNodeContent(nameNode, *sourceBytes)
	} else if c.isConstructorKind(kindStr) && currentQNPrefix != "" {
		parts := strings.Split(currentQNPrefix, ".")
		name = parts[len(parts)-1]
	}

	if name == "" {
		return nil, ""
	}

	return &model.CodeElement{
		Kind:     kind,
		Name:     name,
		Path:     filePath,
		Location: c.extractLocation(node, filePath),
		Doc:      c.extractComments(node, sourceBytes),
	}, kind
}

func (c *Collector) fillElementExtra(node *sitter.Node, elem *model.CodeElement, kind model.ElementKind, sourceBytes *[]byte) {
	extra := &model.ElementExtra{}
	modifiers, annotations := c.extractModifiersAndAnnotations(node, *sourceBytes)
	extra.Modifiers = modifiers
	extra.Annotations = annotations

	switch kind {
	case model.Class, model.Interface, model.Enum, model.KAnnotation:
		c.fillClassExtra(node, extra, modifiers, sourceBytes)
		elem.Signature = c.extractClassSignature(node, elem.Name, modifiers, *sourceBytes)
	case model.Method:
		c.fillMethodExtra(node, extra, sourceBytes)
		elem.Signature = c.extractMethodSignature(node, *sourceBytes, modifiers)
	case model.Field, model.Variable:
		c.fillFieldExtra(node, extra, modifiers, sourceBytes)
	case model.EnumConstant:
		c.fillEnumConstantExtra(node, extra, sourceBytes)
	}

	if extra.MethodExtra != nil || extra.ClassExtra != nil || extra.FieldExtra != nil ||
		extra.EnumConstantExtra != nil || len(extra.Modifiers) > 0 || len(extra.Annotations) > 0 {
		elem.Extra = extra
	}
}

func (c *Collector) fillMethodExtra(node *sitter.Node, extra *model.ElementExtra, sourceBytes *[]byte) {
	me := &model.MethodExtra{
		IsConstructor: c.isConstructorKind(node.Kind()),
	}
	if tNode := node.ChildByFieldName("type"); tNode != nil {
		me.ReturnType = c.getNodeContent(tNode, *sourceBytes)
	}
	if pNode := node.ChildByFieldName("parameters"); pNode != nil {
		for i := 0; i < int(pNode.NamedChildCount()); i++ {
			me.Parameters = append(me.Parameters, c.getNodeContent(pNode.NamedChild(uint(i)), *sourceBytes))
		}
	}
	if thNode := c.findNamedChildOfType(node, "throws"); thNode != nil {
		c.recursiveCollectTypes(thNode, &me.ThrowsTypes, sourceBytes)
	}
	extra.MethodExtra = me
}

func (c *Collector) extractClassSignature(node *sitter.Node, name string, modifiers []string, sourceBytes []byte) string {
	var sb strings.Builder
	if len(modifiers) > 0 {
		sb.WriteString(strings.Join(modifiers, " "))
		sb.WriteString(" ")
	}
	typeMap := map[string]string{
		"class_declaration":           "class ",
		"record_declaration":          "record ",
		"interface_declaration":       "interface ",
		"enum_declaration":            "enum ",
		"annotation_type_declaration": "@interface ",
	}
	sb.WriteString(typeMap[node.Kind()])
	sb.WriteString(name)
	if tpNode := node.ChildByFieldName("type_parameters"); tpNode != nil {
		sb.WriteString(c.getNodeContent(tpNode, sourceBytes))
	}
	if node.Kind() == "record_declaration" {
		if pNode := node.ChildByFieldName("parameters"); pNode != nil {
			sb.WriteString(c.getNodeContent(pNode, sourceBytes))
		}
	}
	return strings.TrimSpace(sb.String())
}

func (c *Collector) extractMethodSignature(node *sitter.Node, sourceBytes []byte, modifiers []string) string {
	var sb strings.Builder
	if len(modifiers) > 0 {
		sb.WriteString(strings.Join(modifiers, " "))
		sb.WriteString(" ")
	}
	if node.Kind() == "compact_constructor_declaration" {
		if nNode := node.ChildByFieldName("name"); nNode != nil {
			sb.WriteString(c.getNodeContent(nNode, sourceBytes))
		}
		sb.WriteString(" {compact}")
		return strings.TrimSpace(sb.String())
	}
	if tpNode := node.ChildByFieldName("type_parameters"); tpNode != nil {
		sb.WriteString(c.getNodeContent(tpNode, sourceBytes) + " ")
	}
	if tNode := node.ChildByFieldName("type"); tNode != nil {
		sb.WriteString(c.getNodeContent(tNode, sourceBytes) + " ")
	}
	if nNode := node.ChildByFieldName("name"); nNode != nil {
		sb.WriteString(c.getNodeContent(nNode, sourceBytes))
	}
	if pNode := node.ChildByFieldName("parameters"); pNode != nil {
		sb.WriteString(c.getNodeContent(pNode, sourceBytes))
	} else {
		if node.Kind() == "annotation_type_element_declaration" || node.Kind() != "constructor_declaration" {
			sb.WriteString("()")
		}
	}
	return strings.TrimSpace(sb.String())
}

func (c *Collector) fillFieldExtra(node *sitter.Node, extra *model.ElementExtra, modifiers []string, sourceBytes *[]byte) {
	fe := &model.FieldExtra{}
	if tNode := node.ChildByFieldName("type"); tNode != nil {
		fe.Type = c.getNodeContent(tNode, *sourceBytes)
	}
	fe.IsConstant = false
	for _, m := range modifiers {
		if m == "final" {
			fe.IsConstant = true
			break
		}
	}
	extra.FieldExtra = fe
}

func (c *Collector) fillClassExtra(node *sitter.Node, extra *model.ElementExtra, modifiers []string, sourceBytes *[]byte) {
	ce := &model.ClassExtra{}
	for _, m := range modifiers {
		if m == "abstract" {
			ce.IsAbstract = true
		}
		if m == "final" {
			ce.IsFinal = true
		}
	}
	if scNode := node.ChildByFieldName("superclass"); scNode != nil {
		for i := 0; i < int(scNode.ChildCount()); i++ {
			child := scNode.Child(uint(i))
			if child.IsNamed() && child.Kind() != "extends" {
				ce.SuperClass = c.getNodeContent(child, *sourceBytes)
				break
			}
		}
	}
	var iNode *sitter.Node
	if n := node.ChildByFieldName("interfaces"); n != nil {
		iNode = n
	} else {
		iNode = c.findNamedChildOfType(node, "extends_interfaces")
	}
	if iNode != nil {
		c.recursiveCollectTypes(iNode, &ce.ImplementedInterfaces, sourceBytes)
	}
	extra.ClassExtra = ce
}

func (c *Collector) fillEnumConstantExtra(node *sitter.Node, extra *model.ElementExtra, sourceBytes *[]byte) {
	argListNode := node.ChildByFieldName("arguments")
	if argListNode == nil {
		return
	}
	ece := &model.EnumConstantExtra{Arguments: make([]string, 0)}
	for i := 0; i < int(argListNode.ChildCount()); i++ {
		child := argListNode.Child(uint(i))
		if child.IsNamed() {
			ece.Arguments = append(ece.Arguments, c.getNodeContent(child, *sourceBytes))
		}
	}
	extra.EnumConstantExtra = ece
}

func (c *Collector) recursiveCollectTypes(n *sitter.Node, results *[]string, sourceBytes *[]byte) {
	kind := n.Kind()
	typeKinds := map[string]bool{
		"type_identifier": true, "scoped_type_identifier": true, "generic_type": true,
		"void_type": true, "integral_type": true, "floating_point_type": true,
		"boolean_type": true, "wildcard": true,
	}
	if typeKinds[kind] {
		*results = append(*results, c.getNodeContent(n, *sourceBytes))
		return
	}
	for i := 0; i < int(n.ChildCount()); i++ {
		c.recursiveCollectTypes(n.Child(uint(i)), results, sourceBytes)
	}
}

func (c *Collector) extractComments(node *sitter.Node, sourceBytes *[]byte) string {
	var comments []string
	curr := node.PrevSibling()
	for curr != nil {
		k := curr.Kind()
		if k == "block_comment" || k == "line_comment" {
			comments = append([]string{c.getNodeContent(curr, *sourceBytes)}, comments...)
		} else if k != "modifiers" && !strings.Contains(k, "annotation") {
			break
		}
		curr = curr.PrevSibling()
	}
	return strings.Join(comments, "\n")
}

// --- 基础工具逻辑 ---

func (c *Collector) getTypeString(param *sitter.Node, sourceBytes *[]byte) string {
	if tNode := param.ChildByFieldName("type"); tNode != nil {
		return c.getNodeContent(tNode, *sourceBytes)
	} else if param.Kind() == "spread_parameter" && param.NamedChildCount() > 0 {
		return c.getNodeContent(param.NamedChild(0), *sourceBytes)
	}
	return "unknown"
}

func (c *Collector) findParentRecord(node *sitter.Node) *sitter.Node {
	curr := node.Parent()
	for curr != nil && curr.Kind() != "record_declaration" {
		curr = curr.Parent()
	}
	return curr
}

func (c *Collector) hasEllipsis(n *sitter.Node) bool {
	for i := 0; i < int(n.ChildCount()); i++ {
		if n.Child(uint(i)).Kind() == "..." {
			return true
		}
	}
	return false
}

func (c *Collector) isRecordComponent(node *sitter.Node) bool {
	parent := node.Parent()
	if parent != nil && parent.Kind() == "formal_parameters" {
		grand := parent.Parent()
		return grand != nil && grand.Kind() == "record_declaration"
	}
	return false
}

func (c *Collector) extractModifiersAndAnnotations(n *sitter.Node, sourceBytes []byte) ([]string, []string) {
	var mods, annos []string
	mNode := n.ChildByFieldName("modifiers")
	if mNode == nil {
		mNode = c.findNamedChildOfType(n, "modifiers")
	}
	if mNode != nil {
		for i := 0; i < int(mNode.ChildCount()); i++ {
			child := mNode.Child(uint(i))
			txt := c.getNodeContent(child, sourceBytes)
			if strings.Contains(child.Kind(), "annotation") {
				annos = append(annos, txt)
			} else if txt != "" {
				mods = append(mods, txt)
			}
		}
	}
	return mods, annos
}

func (c *Collector) getNodeContent(n *sitter.Node, sourceBytes []byte) string {
	if n == nil {
		return ""
	}
	return n.Utf8Text(sourceBytes)
}

func (c *Collector) extractLocation(n *sitter.Node, filePath string) *model.Location {
	if n == nil {
		return nil
	}
	return &model.Location{
		FilePath:  filePath,
		StartLine: int(n.StartPosition().Row) + 1, EndLine: int(n.EndPosition().Row) + 1,
		StartColumn: int(n.StartPosition().Column), EndColumn: int(n.EndPosition().Column),
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

func (c *Collector) isContainerKind(k model.ElementKind) bool {
	return k == model.Class || k == model.Interface || k == model.Enum || k == model.KAnnotation
}

func (c *Collector) isConstructorKind(kind string) bool {
	return kind == "constructor_declaration" || kind == "compact_constructor_declaration"
}
