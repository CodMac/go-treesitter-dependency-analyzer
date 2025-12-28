package java

import (
	"strings"

	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

type Collector struct{}

func NewJavaCollector() *Collector {
	return &Collector{}
}

func (c *Collector) CollectDefinitions(rootNode *sitter.Node, filePath string, sourceBytes *[]byte) (*model.FileContext, error) {
	fCtx := model.NewFileContext(filePath, rootNode, sourceBytes)

	// 1. 处理顶级声明 (Package & Imports)
	c.processTopLevelDeclarations(fCtx)

	// 2. 递归收集定义
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
			content := c.getNodeContent(child, *fCtx.SourceBytes)
			pathParts = append(pathParts, content)
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

	alias := ""
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

func (c *Collector) collectDefinitionsRecursive(node *sitter.Node, fCtx *model.FileContext, currentQNPrefix string) error {
	if node.IsNamed() {
		elem, kind := c.getDefinitionElement(node, fCtx.SourceBytes, fCtx.FilePath, currentQNPrefix)
		if elem != nil {
			parentQN := currentQNPrefix
			elem.QualifiedName = model.BuildQualifiedName(parentQN, elem.Name)
			fCtx.AddDefinition(elem, parentQN)

			// --- 核心优化：处理 Record 组件的隐式 Accessor 方法 ---
			if node.Kind() == "formal_parameter" {
				if parent := node.Parent(); parent != nil && parent.Kind() == "formal_parameters" {
					if grand := parent.Parent(); grand != nil && grand.Kind() == "record_declaration" {
						methodElem := *elem
						methodElem.Kind = model.Method
						methodElem.Signature = elem.Name + "()"
						fCtx.AddDefinition(&methodElem, parentQN)
					}
				}
			}

			// 只有 Container 类型或 Method 需要作为后续子节点的 QN 前缀
			if c.isContainerKind(kind) || kind == model.Method {
				currentQNPrefix = elem.QualifiedName
			}
		}
	}

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
	case "formal_parameter":
		// Record 组件判定
		if node.Parent() != nil && node.Parent().Parent() != nil &&
			node.Parent().Parent().Kind() == "record_declaration" {
			kind = model.Field
		} else {
			return nil, ""
		}
	default:
		return nil, ""
	}

	// 针对不同类型的节点提取名称
	if kindStr == "field_declaration" {
		if vNode := c.findNamedChildOfType(node, "variable_declarator"); vNode != nil {
			nameNode = vNode.ChildByFieldName("name")
		}
	} else {
		nameNode = node.ChildByFieldName("name")
	}

	name := ""
	if nameNode != nil {
		name = c.getNodeContent(nameNode, *sourceBytes)
	} else if (kindStr == "constructor_declaration" || kindStr == "compact_constructor_declaration") && currentQNPrefix != "" {
		// 构造函数名自动补全
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
	case model.Field:
		c.fillFieldExtra(node, extra, modifiers, sourceBytes)
	case model.EnumConstant:
		c.fillEnumConstantExtra(node, extra, sourceBytes)
	}

	if extra.MethodExtra != nil || extra.ClassExtra != nil || extra.FieldExtra != nil ||
		extra.EnumConstantExtra != nil || len(extra.Modifiers) > 0 || len(extra.Annotations) > 0 {
		elem.Extra = extra
	}
}

func (c *Collector) extractClassSignature(node *sitter.Node, name string, modifiers []string, sourceBytes []byte) string {
	var sb strings.Builder
	if len(modifiers) > 0 {
		sb.WriteString(strings.Join(modifiers, " "))
		sb.WriteString(" ")
	}

	switch node.Kind() {
	case "class_declaration":
		sb.WriteString("class ")
	case "record_declaration":
		sb.WriteString("record ")
	case "interface_declaration":
		sb.WriteString("interface ")
	case "enum_declaration":
		sb.WriteString("enum ")
	case "annotation_type_declaration":
		sb.WriteString("@interface ")
	}

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

	// 1. 处理紧凑构造函数
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

	// 2. 核心修复：处理参数括号
	if pNode := node.ChildByFieldName("parameters"); pNode != nil {
		sb.WriteString(c.getNodeContent(pNode, sourceBytes))
	} else {
		// 只有以下情况不自动补全 ():
		// - 构造函数 (在某些 AST 转换中可能暂时没有 parameters 节点)
		// - 注解属性定义 (已经在 signature 中包含了 name)
		// 但是，测试案例期待 "level()"，所以我们需要为注解属性显式加上 ()
		if node.Kind() == "annotation_type_element_declaration" {
			sb.WriteString("()")
		} else if node.Kind() != "constructor_declaration" {
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
	if node.Kind() == "formal_parameter" {
		fe.IsConstant = true
	} else {
		for _, m := range modifiers {
			if m == "final" {
				fe.IsConstant = true
				break
			}
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
		for i := 0; i < int(node.ChildCount()); i++ {
			if node.Child(uint(i)).Kind() == "extends_interfaces" {
				iNode = node.Child(uint(i))
				break
			}
		}
	}
	if iNode != nil {
		c.recursiveCollectTypes(iNode, &ce.ImplementedInterfaces, sourceBytes)
	}
	extra.ClassExtra = ce
}

func (c *Collector) fillMethodExtra(node *sitter.Node, extra *model.ElementExtra, sourceBytes *[]byte) {
	me := &model.MethodExtra{
		IsConstructor: node.Kind() == "constructor_declaration" || node.Kind() == "compact_constructor_declaration",
	}
	if tNode := node.ChildByFieldName("type"); tNode != nil {
		me.ReturnType = c.getNodeContent(tNode, *sourceBytes)
	}
	if pNode := node.ChildByFieldName("parameters"); pNode != nil {
		for i := 0; i < int(pNode.NamedChildCount()); i++ {
			me.Parameters = append(me.Parameters, c.getNodeContent(pNode.NamedChild(uint(i)), *sourceBytes))
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "throws" {
			c.recursiveCollectTypes(child, &me.ThrowsTypes, sourceBytes)
			break
		}
	}
	extra.MethodExtra = me
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

func (c *Collector) extractModifiersAndAnnotations(n *sitter.Node, sourceBytes []byte) ([]string, []string) {
	var mods, annos []string
	mNode := n.ChildByFieldName("modifiers")
	if mNode == nil {
		for i := 0; i < int(n.ChildCount()); i++ {
			if n.Child(uint(i)).Kind() == "modifiers" {
				mNode = n.Child(uint(i))
				break
			}
		}
	}
	if mNode != nil {
		for i := 0; i < int(mNode.ChildCount()); i++ {
			child := mNode.Child(uint(i))
			txt := c.getNodeContent(child, sourceBytes)
			if child.Kind() == "marker_annotation" || child.Kind() == "annotation" {
				annos = append(annos, txt)
			} else if txt != "" {
				mods = append(mods, txt)
			}
		}
	}
	return mods, annos
}

func (c *Collector) recursiveCollectTypes(n *sitter.Node, results *[]string, sourceBytes *[]byte) {
	kind := n.Kind()
	if kind == "type_identifier" || kind == "scoped_type_identifier" || kind == "generic_type" ||
		kind == "void_type" || kind == "integral_type" || kind == "floating_point_type" ||
		kind == "boolean_type" || kind == "wildcard" {
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
		} else if k != "modifiers" && k != "marker_annotation" && k != "annotation" {
			break
		}
		curr = curr.PrevSibling()
	}
	return strings.Join(comments, "\n")
}

func (c *Collector) extractLocation(n *sitter.Node, filePath string) *model.Location {
	if n == nil {
		return nil
	}
	return &model.Location{
		FilePath:    filePath,
		StartLine:   int(n.StartPosition().Row) + 1,
		EndLine:     int(n.EndPosition().Row) + 1,
		StartColumn: int(n.StartPosition().Column),
		EndColumn:   int(n.EndPosition().Column),
	}
}

func (c *Collector) getNodeContent(n *sitter.Node, sourceBytes []byte) string {
	if n == nil {
		return ""
	}
	return n.Utf8Text(sourceBytes)
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
