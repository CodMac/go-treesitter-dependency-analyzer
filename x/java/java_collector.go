package java

import (
	"strings"

	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Collector 实现了 collector.Collector 接口
type Collector struct{}

func NewJavaCollector() *Collector {
	return &Collector{}
}

// CollectDefinitions 实现了 extractor.DefinitionCollector 接口
func (c *Collector) CollectDefinitions(rootNode *sitter.Node, filePath string, sourceBytes *[]byte) (*model.FileContext, error) {
	fCtx := model.NewFileContext(filePath, rootNode, sourceBytes)

	// 1. 独立处理 Package Name (作为 QN 的前缀)
	c.collectPackageName(fCtx)

	// 2. 递归收集定义
	// 初始 QN Stack 只包含 PackageName（如果存在）
	initialQN := ""
	if fCtx.PackageName != "" {
		initialQN = fCtx.PackageName
	}

	if err := c.collectDefinitionsRecursive(fCtx.RootNode, fCtx, initialQN); err != nil {
		return nil, err
	}

	return fCtx, nil
}

// collectPackageName 独立收集 Package Name
func (c *Collector) collectPackageName(fCtx *model.FileContext) {
	var pkgNode *sitter.Node

	// package_declaration 是 program 的非命名子节点，遍历根节点查找。
	for i := 0; i < int(fCtx.RootNode.ChildCount()); i++ {
		child := fCtx.RootNode.Child(uint(i))
		if child != nil && child.Kind() == "package_declaration" {
			pkgNode = child
			break
		}
	}

	if pkgNode != nil {
		// package_declaration 的命名子节点是包名 (scoped_identifier/identifier), 没有 field name "name", 应该直接获取第一个命名子节点 (索引 0)。
		if pkgNameNode := pkgNode.NamedChild(0); pkgNameNode != nil {
			fCtx.PackageName = getNodeContent(pkgNameNode, *fCtx.SourceBytes)
		}
	}
}

// collectDefinitionsRecursive 使用递归来简化 QN 栈的管理。
func (c *Collector) collectDefinitionsRecursive(node *sitter.Node, fCtx *model.FileContext, currentQNPrefix string) error {
	// 检查当前节点是否是一个定义
	if elem, kind := getDefinitionElement(node, fCtx.SourceBytes, fCtx.FilePath); elem != nil {
		// 1. 构造 Qualified Name
		parentQN := currentQNPrefix
		elem.QualifiedName = model.BuildQualifiedName(parentQN, elem.Name)

		// 2. 注册定义
		fCtx.AddDefinition(elem, parentQN)

		// 3. 更新 QN 前缀
		// 对于 Class/Interface/Enum/Method，它们是容器，新的 QN 是其自身的 QualifiedName
		if kind == model.Class || kind == model.Interface || kind == model.Enum || kind == model.Method {
			currentQNPrefix = elem.QualifiedName
		}
	}

	// 递归遍历子节点
	cursor := node.Walk()
	defer cursor.Close()

	if cursor.GotoFirstChild() {
		for {
			// 递归调用，并传入当前 QN 前缀
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

// extractLocation 从 sitter.Node 中提取位置信息
func extractLocation(n *sitter.Node, filePath string) *model.Location {
	if n == nil {
		return nil
	}
	return &model.Location{
		FilePath:    filePath,
		StartLine:   int(n.StartPosition().Row) + 1, // 行号从 1 开始
		EndLine:     int(n.EndPosition().Row) + 1,
		StartColumn: int(n.StartPosition().Column),
		EndColumn:   int(n.EndPosition().Column),
	}
}

// extractModifiers 从节点中提取修饰符列表。 仅检查 "modifiers" 命名子节点 (Java)
func extractModifiers(n *sitter.Node, sourceBytes []byte) []string {
	var modifiers []string
	// 查找 modifiers 节点
	modifiersNode := n.ChildByFieldName("modifiers")
	if modifiersNode == nil {
		return modifiers
	}

	// 遍历 modifiers 节点下的所有子节点，只提取关键字修饰符
	for i := 0; i < int(modifiersNode.ChildCount()); i++ {
		child := modifiersNode.Child(uint(i))
		childType := child.Kind()
		if childType == "public" || childType == "private" || childType == "protected" ||
			childType == "static" || childType == "final" || childType == "abstract" ||
			childType == "synchronized" || childType == "transient" || childType == "volatile" ||
			childType == "default" {

			modifiers = append(modifiers, getNodeContent(child, sourceBytes))
		}
		// 忽略 Annotation
	}

	return modifiers
}

// extractMethodSignature 提取方法或构造函数的签名字符串
func extractMethodSignature(node *sitter.Node, sourceBytes []byte) string {
	var parts []string

	// 1. 修饰符 (可选)
	modifiers := extractModifiers(node, sourceBytes)
	if len(modifiers) > 0 {
		parts = append(parts, strings.Join(modifiers, " "))
	}

	// 2. 返回类型 (对于 method_declaration)
	if node.Kind() == "method_declaration" {
		if typeNode := node.ChildByFieldName("type"); typeNode != nil {
			parts = append(parts, getNodeContent(typeNode, sourceBytes))
		}
	}

	// 3. 名称 (method_declaration/constructor_declaration)
	name := ""
	nameNode := node.ChildByFieldName("name")
	if nameNode != nil {
		name = getNodeContent(nameNode, sourceBytes)
	} else if node.Kind() == "constructor_declaration" {
		// 构造函数名称与其父类名称相同
		if parent := node.Parent(); parent != nil {
			if classNameNode := parent.ChildByFieldName("name"); classNameNode != nil {
				name = getNodeContent(classNameNode, sourceBytes)
			}
		}
	}

	if name != "" {
		parts = append(parts, name)
	}

	// 4. 参数列表 (formal_parameters)
	if paramsNode := node.ChildByFieldName("parameters"); paramsNode != nil {
		parts = append(parts, getNodeContent(paramsNode, sourceBytes))
	} else if node.Kind() == "method_declaration" || node.Kind() == "constructor_declaration" {
		// 没有参数的函数/方法也需要添加 ()
		parts = append(parts, "()")
	}

	// 5. Throws 子句 (可选)
	if throwsNode := node.ChildByFieldName("throws"); throwsNode != nil {
		parts = append(parts, getNodeContent(throwsNode, sourceBytes))
	}

	return strings.Join(parts, " ")
}

// getDefinitionElement 辅助函数
func getDefinitionElement(node *sitter.Node, sourceBytes *[]byte, filePath string) (*model.CodeElement, model.ElementKind) {
	if node == nil {
		return nil, ""
	}

	var elem *model.CodeElement
	var kind model.ElementKind
	var nameNode *sitter.Node

	// 1. 尝试获取 NameNode 和 Kind
	switch node.Kind() {
	case "class_declaration":
		nameNode = node.ChildByFieldName("name")
		kind = model.Class
	case "interface_declaration":
		nameNode = node.ChildByFieldName("name")
		kind = model.Interface
	case "enum_declaration":
		nameNode = node.ChildByFieldName("name")
		kind = model.Enum
	case "enum_constant":
		nameNode = node.ChildByFieldName("name")
		kind = model.EnumConstant
	case "method_declaration", "constructor_declaration":
		nameNode = node.ChildByFieldName("name")
		kind = model.Method
	case "field_declaration":
		// 字段声明可能包含多个变量声明，我们只取第一个
		if vNode := findNamedChildOfType(node, "variable_declarator"); vNode != nil {
			nameNode = vNode.ChildByFieldName("name")
			kind = model.Field
		}
	}

	if nameNode == nil && (kind == model.Class || kind == model.Interface || kind == model.Enum || kind == model.Field) {
		return nil, ""
	}

	// 处理 Name 和 Method 构造函数名称的特殊情况
	name := ""
	if nameNode != nil {
		name = getNodeContent(nameNode, *sourceBytes)
	} else if node.Kind() == "constructor_declaration" {
		// 构造函数名称与其父类/父枚举名称相同
		if parent := node.Parent(); parent != nil && (parent.Kind() == "class_declaration" || parent.Kind() == "enum_declaration") {
			if classNameNode := parent.ChildByFieldName("name"); classNameNode != nil {
				name = getNodeContent(classNameNode, *sourceBytes)
			}
		}
		if name == "" {
			name = "Constructor" // 默认回退名称
		}
	}

	if name == "" {
		return nil, ""
	}

	// 2. 构造 CodeElement 基础信息
	elem = &model.CodeElement{
		Kind: kind,
		Name: name,
		Path: filePath,
	}

	// 3. 收集 Location
	elem.Location = extractLocation(node, filePath)

	// 4. 收集 Signature 和 Extra 信息
	extra := &model.ElementExtra{}
	modifiers := extractModifiers(node, *sourceBytes)
	if len(modifiers) > 0 {
		extra.Modifiers = modifiers
	}

	switch kind {
	case model.Class, model.Interface, model.Enum:
		// 收集 Class/Interface/Enum Extra
		classExtra := &model.ClassExtra{}

		if node.Kind() == "class_declaration" {
			// SuperClass
			if extendsNode := node.ChildByFieldName("superclass"); extendsNode != nil {
				classExtra.SuperClass = getNodeContent(extendsNode.NamedChild(0), *sourceBytes)
			}
			// Implemented Interfaces
			if interfacesNode := node.ChildByFieldName("interfaces"); interfacesNode != nil {
				if typeListNode := interfacesNode.NamedChild(0); typeListNode != nil {
					for i := 0; i < int(typeListNode.NamedChildCount()); i++ {
						classExtra.ImplementedInterfaces = append(classExtra.ImplementedInterfaces, getNodeContent(typeListNode.NamedChild(uint(i)), *sourceBytes))
					}
				}
			}
			// 提取 isAbstract/isFinal (基于 Modifiers)
			for _, mod := range modifiers {
				if mod == "abstract" {
					classExtra.IsAbstract = true
				}
				if mod == "final" {
					classExtra.IsFinal = true
				}
			}
		} else if node.Kind() == "interface_declaration" {
			// Interface 继承 (extends/superinterfaces)
			if superInterfacesNode := node.ChildByFieldName("superinterfaces"); superInterfacesNode != nil {
				if typeListNode := superInterfacesNode.NamedChild(0); typeListNode != nil {
					for i := 0; i < int(typeListNode.NamedChildCount()); i++ {
						// 接口继承的列表存放在 ImplementedInterfaces 字段中
						classExtra.ImplementedInterfaces = append(classExtra.ImplementedInterfaces, getNodeContent(typeListNode.NamedChild(uint(i)), *sourceBytes))
					}
				}
			}
		}
		extra.ClassExtra = classExtra

	case model.Method:
		// 收集 Method Extra 和 Signature
		elem.Signature = extractMethodSignature(node, *sourceBytes)
		methodExtra := &model.MethodExtra{
			IsConstructor: node.Kind() == "constructor_declaration",
		}

		// 提取 ReturnType (对于 method_declaration)
		if node.Kind() == "method_declaration" {
			if typeNode := node.ChildByFieldName("type"); typeNode != nil {
				extra.ReturnType = getNodeContent(typeNode, *sourceBytes)
			}
		}

		// 提取 Throws
		if throwsNode := node.ChildByFieldName("throws"); throwsNode != nil {
			if typeListNode := throwsNode.NamedChild(0); typeListNode != nil {
				for i := 0; i < int(typeListNode.NamedChildCount()); i++ {
					methodExtra.ThrowsTypes = append(methodExtra.ThrowsTypes, getNodeContent(typeListNode.NamedChild(uint(i)), *sourceBytes))
				}
			}
		}

		// 提取 Parameters 列表
		if paramsNode := node.ChildByFieldName("parameters"); paramsNode != nil {
			if formalParamsNode := paramsNode.NamedChild(0); formalParamsNode != nil {
				for i := 0; i < int(formalParamsNode.NamedChildCount()); i++ {
					// 每个 formal_parameter 是一个参数声明
					methodExtra.Parameters = append(methodExtra.Parameters, getNodeContent(formalParamsNode.NamedChild(uint(i)), *sourceBytes))
				}
			}
		}
		extra.MethodExtra = methodExtra

	case model.Field:
		// 收集 Field Extra
		fieldExtra := &model.FieldExtra{}

		// 提取 Type
		if typeNode := node.ChildByFieldName("type"); typeNode != nil {
			extra.Type = getNodeContent(typeNode, *sourceBytes)
		}

		// 提取 IsConstant (Java: final field)
		for _, mod := range modifiers {
			if mod == "final" {
				fieldExtra.IsConstant = true
				break
			}
		}

		extra.FieldExtra = fieldExtra

	case model.EnumConstant:
		// 收集 EnumConstant Signature (如果有参数)
		elem.Signature = name
		if argsNode := node.ChildByFieldName("arguments"); argsNode != nil {
			// 如果有参数，签名是 NAME(...)
			elem.Signature += getNodeContent(argsNode, *sourceBytes)
		}
	}

	elem.Extra = extra

	return elem, kind
}

// getNodeContent 获取 AST 节点对应的源码文本内容
func getNodeContent(n *sitter.Node, sourceBytes []byte) string {
	if n == nil {
		return ""
	}
	return n.Utf8Text(sourceBytes)
}

// findNamedChildOfType 查找特定类型的命名子节点
func findNamedChildOfType(n *sitter.Node, nodeType string) *sitter.Node {
	if n == nil {
		return nil
	}

	for i := 0; i < int(n.NamedChildCount()); i++ {
		child := n.NamedChild(uint(i))
		if child != nil && child.Kind() == nodeType {
			return child
		}
	}
	return nil
}
