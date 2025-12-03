package java

import (
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
	if err := c.collectDefinitionsLogic(fCtx); err != nil {
		return nil, err
	}

	return fCtx, nil
}

func (c *Collector) collectDefinitionsLogic(fCtx *model.FileContext) error {
	cursor := fCtx.RootNode.Walk()
	defer cursor.Close()

	qnStack := []string{}

	// 1. 预处理 Package Name
	if pkgNode := fCtx.RootNode.ChildByFieldName("package_declaration"); pkgNode != nil && pkgNode.ChildCount() > 1 {
		if pkgNameNode := pkgNode.Child(1); pkgNameNode != nil {
			fCtx.PackageName = getNodeContent(pkgNameNode, *fCtx.SourceBytes)
			qnStack = append(qnStack, fCtx.PackageName)
		}
	}

	// 2. 深度优先遍历 AST
	if cursor.GotoFirstChild() {
		for {
			node := cursor.Node()

			if elem, kind := getDefinitionElement(node, fCtx.SourceBytes, fCtx.FilePath); elem != nil {
				parentQN := ""
				if len(qnStack) > 0 {
					parentQN = qnStack[len(qnStack)-1]
				}

				if kind != model.File && kind != model.Package {
					elem.QualifiedName = model.BuildQualifiedName(parentQN, elem.Name)
				} else {
					elem.QualifiedName = elem.Name
				}

				fCtx.AddDefinition(elem, parentQN)

				if kind == model.Class || kind == model.Interface || kind == model.Method {
					qnStack = append(qnStack, elem.QualifiedName)
				}

				if cursor.GotoFirstChild() {
					continue
				}
			}

			if cursor.GotoNextSibling() {
				continue
			}

			for {
				if _, kind := getDefinitionElement(cursor.Node(), fCtx.SourceBytes, fCtx.FilePath); kind == model.Class || kind == model.Interface || kind == model.Method {
					if len(qnStack) > 0 {
						qnStack = qnStack[:len(qnStack)-1]
					}
				}

				if cursor.GotoParent() {
					if cursor.GotoNextSibling() {
						break
					}
				} else {
					return nil
				}
			}
		}
	}
	return nil
}

// getDefinitionElement 辅助函数
func getDefinitionElement(node *sitter.Node, sourceBytes *[]byte, filePath string) (*model.CodeElement, model.ElementKind) {
	if node == nil {
		return nil, ""
	}

	switch node.Kind() {
	case "class_declaration":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			return &model.CodeElement{Kind: model.Class, Name: getNodeContent(nameNode, *sourceBytes), Path: filePath}, model.Class
		}
	case "interface_declaration":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			return &model.CodeElement{Kind: model.Interface, Name: getNodeContent(nameNode, *sourceBytes), Path: filePath}, model.Interface
		}
	case "method_declaration", "constructor_declaration":
		nameNode := node.ChildByFieldName("name")
		name := ""
		if nameNode != nil {
			name = getNodeContent(nameNode, *sourceBytes)
		} else if node.Kind() == "constructor_declaration" {
			if parent := node.Parent(); parent != nil && parent.Kind() == "class_declaration" {
				if classNameNode := parent.ChildByFieldName("name"); classNameNode != nil {
					name = getNodeContent(classNameNode, *sourceBytes)
				}
			}
			if name == "" {
				name = "Constructor"
			}
		}

		if name != "" {
			return &model.CodeElement{Kind: model.Method, Name: name, Path: filePath}, model.Method
		}
	case "field_declaration":
		if vNode := findNamedChildOfType(node, "variable_declarator"); vNode != nil {
			if nameNode := vNode.ChildByFieldName("name"); nameNode != nil {
				return &model.CodeElement{Kind: model.Field, Name: getNodeContent(nameNode, *sourceBytes), Path: filePath}, model.Field
			}
		}
	}
	return nil, ""
}

// getNodeContent 获取 AST 节点对应的源码文本内容
func getNodeContent(n *sitter.Node, sourceBytes []byte) string {
	if n == nil {
		return ""
	}
	// Utf8Text 存在，接受 []byte 参数
	return n.Utf8Text(sourceBytes)
}

// findNamedChildOfType 查找特定类型的命名子节点
func findNamedChildOfType(n *sitter.Node, nodeType string) *sitter.Node {
	if n == nil {
		return nil
	}

	// NamedChildCount() 和 NamedChild(i) 存在
	for i := 0; i < int(n.NamedChildCount()); i++ {
		child := n.NamedChild(uint(i))
		if child != nil && child.Kind() == nodeType {
			return child
		}
	}
	return nil
}
