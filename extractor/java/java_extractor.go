package java

import (
	"fmt"
	"github.com/CodMac/go-treesitter-dependency-analyzer/extractor"
	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	"github.com/CodMac/go-treesitter-dependency-analyzer/parser"
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

// JavaExtractor 实现了 extractor.Extractor 接口
type JavaExtractor struct{}

func NewJavaExtractor() *JavaExtractor {
	return &JavaExtractor{}
}

func init() {
	// 注册 Tree-sitter Java 语言对象
	parser.RegisterLanguage(model.LangJava, sitter.NewLanguage(tree_sitter_java.Language()))
	// 注册 Extractor 工厂函数
	extractor.RegisterExtractor(model.Language("java"), func() extractor.Extractor {
		return NewJavaExtractor()
	})
}

// --- Tree-sitter Queries ---

const (
	// JavaDefinitionQuery: 收集定义和结构关系 (CONTAIN, EXTEND, IMPLEMENT)
	JavaDefinitionQuery = `
		(program 
			[
				(package_declaration (scoped_identifier) @package_name) @package_def
				(import_declaration (scoped_identifier) @import_target) @import_def
				(class_declaration
					name: (identifier) @class_name
					(type_parameters)?
					(superclass (identifier) @extends_class)?
					(super_interfaces (type_list (identifier) @implements_interface+))?
					body: (class_body 
						(field_declaration (variable_declarator (identifier) @field_name)) @field_def
						(method_declaration
							type: (_) @return_type 
							name: (identifier) @method_name
							parameters: (formal_parameters (formal_parameter type: (_) @param_type) @param_node+)?
							(throws (scoped_type_identifier) @throws_type)?
						) @method_def
						(constructor_declaration 
							name: (identifier) @constructor_name
							parameters: (formal_parameters (formal_parameter type: (_) @param_type) @param_node+)?
							(throws (scoped_type_identifier) @throws_type)?
						) @constructor_def
					)
					(modifiers (annotation name: (identifier) @annotation_name)) @annotation_stmt
				) @class_def
				(interface_declaration
					name: (identifier) @interface_name
					(super_interfaces (type_list (identifier) @extends_interface+))?
					(modifiers (annotation name: (identifier) @annotation_name)) @annotation_stmt
				) @interface_def
			]
		)
	`
	// JavaRelationQuery: 收集操作关系 (CALL, CREATE, USE, CAST, ANNOTATION)
	JavaRelationQuery = `
		[
			; 1. 方法调用 (CALL)
			(method_invocation name: (identifier) @call_target) @call_stmt
			
			; 2. 对象创建 (CREATE)
			(object_creation_expression type: (unqualified_class_instance_expression type: (identifier) @create_target_name)) @create_stmt
			
			; 3. 字段/变量读取 (USE)
			(field_access field: (identifier) @use_field_name) @use_field_stmt

			; 4. 显式类型转换 (CAST)
			(cast_expression type: (_) @cast_type) @cast_stmt
			
			; 5. 通用标识符引用 (USE)
			(identifier) @use_identifier
			
			; 6. 独立注解 (ANNOTATION) - 针对局部变量或方法体内的表达式
			(local_variable_declaration 
				(modifiers (annotation name: (identifier) @annotation_name)) @annotation_stmt
			)
		]
	`
)

// --- Phase 1: Collect Definitions and Structure Relations ---

// CollectDefinitions 实现了 extractor.DefinitionCollector 接口
func (e *JavaExtractor) CollectDefinitions(rootNode *sitter.Node, filePath string) (*extractor.FileContext, error) {
	ctx := extractor.NewFileContext(filePath)
	if err := e.collectDefinitionsLogic(rootNode, ctx); err != nil {
		return nil, err
	}
	return ctx, nil
}

// collectDefinitionsLogic 封装定义收集和 CONTAIN 关系创建
func (e *JavaExtractor) collectDefinitionsLogic(rootNode *sitter.Node, ctx *extractor.FileContext) error {
	//cursor := sitter.NewTreeCursor(rootNode)
	cursor := rootNode.Walk()
	defer cursor.Close()

	qnStack := []string{}

	// 1. 预处理 Package Name
	if pkgNode := rootNode.ChildByFieldName("package_declaration"); pkgNode != nil && pkgNode.ChildCount() > 1 {
		if pkgNameNode := pkgNode.Child(1); pkgNameNode != nil {
			ctx.PackageName = pkgNameNode.Content()
			qnStack = append(qnStack, ctx.PackageName)
		}
	}

	// 2. 深度优先遍历 AST
	if cursor.GoToFirstChild() {
		for {
			node := cursor.CurrentNode()

			if elem, kind := getDefinitionElement(node, ctx.FilePath); elem != nil {
				parentQN := ""
				if len(qnStack) > 0 {
					parentQN = qnStack[len(qnStack)-1]
				}

				if kind != model.File && kind != model.Package {
					elem.QualifiedName = model.BuildQualifiedName(parentQN, elem.Name)
				} else {
					elem.QualifiedName = elem.Name
				}

				ctx.AddDefinition(elem, parentQN)

				// 创建 CONTAIN 关系
				if parentQN != "" {
					ctx.Definitions[parentQN].Element.Path = ctx.FilePath // 确保父元素有 Path
					rel := &model.DependencyRelation{
						Type:     model.Contain,
						Source:   ctx.Definitions[parentQN].Element,
						Target:   elem,
						Location: nodeToLocation(node, ctx.FilePath),
					}
					// 注意：此处应将关系存储在 FileContext 中，但为简化接口，目前假设关系都在 Phase 2 统一返回。
					// 实际项目中，Phase 1 可能需要返回一个包含 CONTAIN 关系列表的结构。
				}

				if kind == model.Class || kind == model.Interface || kind == model.Method {
					qnStack = append(qnStack, elem.QualifiedName)
				}

				if cursor.GoToFirstChild() {
					continue
				}
			}

			if cursor.GoToNextSibling() {
				continue
			}

			for {
				// 退出容器时，弹出 QN 栈
				if _, kind := getDefinitionElement(cursor.CurrentNode(), ctx.FilePath); kind == model.Class || kind == model.Interface || kind == model.Method {
					if len(qnStack) > 0 {
						qnStack = qnStack[:len(qnStack)-1]
					}
				}

				if cursor.GoToParent() {
					if cursor.GoToNextSibling() {
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

// getDefinitionElement 辅助函数：根据 AST 节点类型，返回对应的 CodeElement 定义和 Kind。
func getDefinitionElement(node *sitter.Node, filePath string) (*model.CodeElement, model.ElementKind) {
	switch node.Type() {
	case "class_declaration":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			return &model.CodeElement{Kind: model.Class, Name: nameNode.Content(), Path: filePath}, model.Class
		}
	case "interface_declaration":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			return &model.CodeElement{Kind: model.Interface, Name: nameNode.Content(), Path: filePath}, model.Interface
		}
	case "method_declaration", "constructor_declaration":
		nameNode := node.ChildByFieldName("name")
		name := ""
		if nameNode != nil {
			name = nameNode.Content()
		} else if node.Type() == "constructor_declaration" {
			// 尝试从父节点获取类名作为构造函数名
			if parent := node.Parent(); parent != nil && parent.Type() == "class_declaration" {
				if classNameNode := parent.ChildByFieldName("name"); classNameNode != nil {
					name = classNameNode.Content()
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
				return &model.CodeElement{Kind: model.Field, Name: nameNode.Content(), Path: filePath}, model.Field
			}
		}
	case "package_declaration":
		// 通常在 collectDefinitionsLogic 顶部处理
		return nil, ""
	}

	return nil, ""
}

// --- Phase 2: Extract Relations ---

// Extract 实现了 extractor.ContextExtractor 接口
func (e *JavaExtractor) Extract(rootNode *sitter.Node, filePath string, gc *model.GlobalContext) ([]*model.DependencyRelation, error) {
	relations := make([]*model.DependencyRelation, 0)
	tsLang, _ := parser.GetLanguage(parser.LangJava)

	// 1. 结构和定义关系 (EXTEND, IMPLEMENT, PARAMETER, RETURN, THROW, ANNOTATION)
	if err := e.processQuery(rootNode, tsLang, JavaDefinitionQuery, filePath, &relations, func(qc *sitter.QueryCursor, match *sitter.QueryMatch, fp string) ([]*model.DependencyRelation, error) {
		return e.handleDefinitionAndStructureRelations(qc, match, fp, gc)
	}); err != nil {
		return nil, fmt.Errorf("failed to process definition query: %w", err)
	}

	// 2. 操作关系 (CALL, CREATE, USE, CAST)
	if err := e.processQuery(rootNode, tsLang, JavaRelationQuery, filePath, &relations, func(qc *sitter.QueryCursor, match *sitter.QueryMatch, fp string) ([]*model.DependencyRelation, error) {
		return e.handleActionRelations(qc, match, fp, gc)
	}); err != nil {
		return nil, fmt.Errorf("failed to process relation query: %w", err)
	}

	return relations, nil
}

// handleDefinitionAndStructureRelations 处理 IMPORT, EXTEND, IMPLEMENT, PARAMETER, RETURN, THROW, ANNOTATION
func (e *JavaExtractor) handleDefinitionAndStructureRelations(qc *sitter.QueryCursor, match *sitter.QueryMatch, filePath string, gc *model.GlobalContext) ([]*model.DependencyRelation, error) {
	relations := make([]*model.DependencyRelation, 0)

	// 获取关系源 (Source)
	var sourceElement *model.CodeElement
	if sourceNode := findStatementNode(match, "class_def", "method_def", "constructor_def"); sourceNode != nil {
		sourceElement = determineSourceElement(sourceNode, filePath, gc)
	} else if importNode := findCapturedNode(match, "import_def"); importNode != nil {
		sourceElement = &model.CodeElement{Kind: model.File, QualifiedName: filePath, Path: filePath}
	}

	if sourceElement == nil {
		// 如果无法确定 Source，则跳过此匹配
		return relations, nil
	}

	// 1. IMPORT
	if importTargetNode := findCapturedNode(match, "import_target"); importTargetNode != nil {
		relations = append(relations, &model.DependencyRelation{
			Type:   model.Import,
			Source: sourceElement,
			Target: &model.CodeElement{
				Kind:          model.Package,
				Name:          importTargetNode.Content(),
				QualifiedName: importTargetNode.Content(), // Import目标通常已是QN
			},
			Location: nodeToLocation(importTargetNode, filePath),
		})
	}

	// 2. EXTEND
	if extendsNode := findCapturedNode(match, "extends_class"); extendsNode != nil {
		relations = append(relations, &model.DependencyRelation{
			Type:   model.Extend,
			Source: sourceElement,
			Target: &model.CodeElement{
				Kind:          model.Class,
				Name:          extendsNode.Content(),
				QualifiedName: resolveQualifiedName(extendsNode, filePath, gc),
			},
			Location: nodeToLocation(extendsNode, filePath),
		})
	}

	// 3. IMPLEMENT
	if implementsNode := findCapturedNode(match, "implements_interface"); implementsNode != nil {
		relations = append(relations, &model.DependencyRelation{
			Type:   model.Implement,
			Source: sourceElement,
			Target: &model.CodeElement{
				Kind:          model.Interface,
				Name:          implementsNode.Content(),
				QualifiedName: resolveQualifiedName(implementsNode, filePath, gc),
			},
			Location: nodeToLocation(implementsNode, filePath),
		})
	}

	// 4. PARAMETER, RETURN, THROW (只在 method/constructor context 中处理)
	if sourceElement.Kind == model.Method {
		// RETURN
		if returnNode := findCapturedNode(match, "return_type"); returnNode != nil && returnNode.Content() != "" && returnNode.Content() != "void" {
			relations = append(relations, &model.DependencyRelation{
				Type:   model.Return,
				Source: sourceElement,
				Target: &model.CodeElement{
					Kind:          model.Type,
					Name:          returnNode.Content(),
					QualifiedName: resolveQualifiedName(returnNode, filePath, gc),
				},
				Location: nodeToLocation(returnNode, filePath),
			})
		}

		// PARAMETER
		if paramNode := findCapturedNode(match, "param_node"); paramNode != nil {
			// 实际实现中，需要遍历所有 formal_parameter 节点
			if paramTypeNode := findNamedChildOfType(paramNode, "identifier"); paramTypeNode != nil {
				relations = append(relations, &model.DependencyRelation{
					Type:   model.Parameter,
					Source: sourceElement,
					Target: &model.CodeElement{
						Kind:          model.Type,
						Name:          paramTypeNode.Content(),
						QualifiedName: resolveQualifiedName(paramTypeNode, filePath, gc),
					},
					Location: nodeToLocation(paramNode, filePath),
				})
			}
		}

		// THROW
		if throwsNode := findCapturedNode(match, "throws_type"); throwsNode != nil {
			relations = append(relations, &model.DependencyRelation{
				Type:   model.Throw,
				Source: sourceElement,
				Target: &model.CodeElement{
					Kind:          model.Type,
					Name:          throwsNode.Content(),
					QualifiedName: resolveQualifiedName(throwsNode, filePath, gc),
				},
				Location: nodeToLocation(throwsNode, filePath),
			})
		}
	}

	// 5. ANNOTATION (针对类/接口)
	if annotationNameNode := findCapturedNode(match, "annotation_name"); annotationNameNode != nil {
		relations = append(relations, &model.DependencyRelation{
			Type:   model.Annotation,
			Source: sourceElement,
			Target: &model.CodeElement{
				Kind:          model.Annotation,
				Name:          annotationNameNode.Content(),
				QualifiedName: resolveQualifiedName(annotationNameNode, filePath, gc),
			},
			Location: nodeToLocation(annotationNameNode, filePath),
		})
	}

	return relations, nil
}

// handleActionRelations 处理 CALL, CREATE, USE, CAST, ANNOTATION (方法体/局部)
func (e *JavaExtractor) handleActionRelations(qc *sitter.QueryCursor, match *sitter.QueryMatch, filePath string, gc *model.GlobalContext) ([]*model.DependencyRelation, error) {
	relations := make([]*model.DependencyRelation, 0)

	// 确定关系的源 (Source): 找到最近的方法/构造函数
	sourceElement := determineSourceElement(match.Captures[0].Node, filePath, gc)
	if sourceElement == nil {
		// 如果在全局找不到方法/构造函数，回退到文件级或跳过
		sourceElement = &model.CodeElement{Kind: model.File, QualifiedName: filePath, Path: filePath}
	}

	// 1. CALL
	if callStmt := findCapturedNode(match, "call_stmt"); callStmt != nil {
		if callTarget := findCapturedNode(match, "call_target"); callTarget != nil {
			relations = append(relations, &model.DependencyRelation{
				Type:   model.Call,
				Source: sourceElement,
				Target: &model.CodeElement{
					Kind:          model.Method,
					Name:          callTarget.Content(),
					QualifiedName: resolveQualifiedName(callTarget, filePath, gc),
				},
				Location: nodeToLocation(callStmt, filePath),
			})
		}
	}

	// 2. CREATE
	if createStmt := findCapturedNode(match, "create_stmt"); createStmt != nil {
		if createTarget := findCapturedNode(match, "create_target_name"); createTarget != nil {
			relations = append(relations, &model.DependencyRelation{
				Type:   model.Create,
				Source: sourceElement,
				Target: &model.CodeElement{
					Kind:          model.Class,
					Name:          createTarget.Content(),
					QualifiedName: resolveQualifiedName(createTarget, filePath, gc),
				},
				Location: nodeToLocation(createStmt, filePath),
			})
		}
	}

	// 3. USE (字段/变量)
	if fieldUseStmt := findCapturedNode(match, "use_field_stmt"); fieldUseStmt != nil {
		if fieldName := findCapturedNode(match, "use_field_name"); fieldName != nil {
			relations = append(relations, &model.DependencyRelation{
				Type:   model.Use,
				Source: sourceElement,
				Target: &model.CodeElement{
					Kind:          model.Field, // 默认字段，精确判断需要类型推导
					Name:          fieldName.Content(),
					QualifiedName: resolveQualifiedName(fieldName, filePath, gc),
				},
				Location: nodeToLocation(fieldUseStmt, filePath),
			})
		}
	}

	// 4. CAST
	if castStmt := findCapturedNode(match, "cast_stmt"); castStmt != nil {
		if castType := findCapturedNode(match, "cast_type"); castType != nil {
			relations = append(relations, &model.DependencyRelation{
				Type:   model.Cast,
				Source: sourceElement,
				Target: &model.CodeElement{
					Kind:          model.Type,
					Name:          castType.Content(),
					QualifiedName: resolveQualifiedName(castType, filePath, gc),
				},
				Location: nodeToLocation(castStmt, filePath),
			})
		}
	}

	// 5. ANNOTATION (局部变量)
	if annotationNameNode := findCapturedNode(match, "annotation_name"); annotationNameNode != nil {
		// 避免与 class/method 上的注解重复，这里捕获的是 local_variable_declaration 内部的
		relations = append(relations, &model.DependencyRelation{
			Type:   model.Annotation,
			Source: sourceElement,
			Target: &model.CodeElement{
				Kind:          model.Annotation,
				Name:          annotationNameNode.Content(),
				QualifiedName: resolveQualifiedName(annotationNameNode, filePath, gc),
			},
			Location: nodeToLocation(annotationNameNode, filePath),
		})
	}

	// 6. USE (简单标识符 - 需谨慎过滤，通常用于类型名)
	if simpleID := findCapturedNode(match, "use_identifier"); simpleID != nil {
		// 简单的启发式过滤，避免与 CALL/CREATE/DEFINE 重复
		parent := simpleID.Parent()
		if parent != nil && (parent.Type() == "method_invocation" ||
			parent.Type() == "class_declaration" ||
			parent.Type() == "field_declaration" ||
			parent.Type() == "import_declaration" ||
			parent.Type() == "package_declaration" ||
			parent.Type() == "type_identifier") {
			return relations, nil
		}

		relations = append(relations, &model.DependencyRelation{
			Type:   model.Use,
			Source: sourceElement,
			Target: &model.CodeElement{
				Kind:          model.Variable,
				Name:          simpleID.Content(),
				QualifiedName: resolveQualifiedName(simpleID, filePath, gc),
			},
			Location: nodeToLocation(simpleID, filePath),
		})
	}

	return relations, nil
}

// --- 通用辅助函数实现 (用于 Tree-sitter 操作和 QN 解析) ---

// processQuery 运行 Tree-sitter 查询并处理匹配项
func (e *JavaExtractor) processQuery(rootNode *sitter.Node, tsLang *sitter.Language, queryStr string, filePath string, relations *[]*model.DependencyRelation, handler func(*sitter.QueryCursor, *sitter.QueryMatch, string) ([]*model.DependencyRelation, error)) error {
	q, err := sitter.NewQuery([]byte(queryStr), tsLang)
	if err != nil {
		return fmt.Errorf("failed to create query: %w", err)
	}
	defer q.Close()

	qc := sitter.NewQueryCursor()
	qc.Exec(q, rootNode)

	for {
		match, ok := qc.NextMatch()
		if !ok {
			break
		}

		// 应用 Predicate 过滤 (可选，但推荐)
		// ...

		// 运行 Handler
		newRelations, err := handler(qc, match, filePath)
		if err != nil {
			return err
		}
		*relations = append(*relations, newRelations...)
	}

	return nil
}

// findCapturedNode 从匹配中查找指定名称的捕获节点
func findCapturedNode(match *sitter.QueryMatch, name string) *sitter.Node {
	for i, captureName := range match.CaptureNames {
		if captureName == name {
			return match.Captures[i].Node
		}
	}
	return nil
}

// findStatementNode 查找匹配到的语句节点 (用于确定 Source Element)
func findStatementNode(match *sitter.QueryMatch, names ...string) *sitter.Node {
	for _, name := range names {
		if node := findCapturedNode(match, name); node != nil {
			return node
		}
	}
	return nil
}

// findNamedChildOfType 查找特定类型的命名子节点
func findNamedChildOfType(n *sitter.Node, nodeType string) *sitter.Node {
	for i := 0; i < int(n.NamedChildCount()); i++ {
		child := n.NamedChild(i)
		if child.Type() == nodeType {
			return child
		}
	}
	return nil
}

// determineSourceElement 向上遍历 AST 查找最近的 Method/Class 作为关系 Source
func determineSourceElement(n *sitter.Node, filePath string, gc *model.GlobalContext) *model.CodeElement {
	// 从当前节点向上查找最近的 Method 或 Class 定义

	cursor := sitter.NewTreeCursor(n)
	defer cursor.Close()

	// 查找 Method/Constructor
	if cursor.GoToParent() {
		for {
			node := cursor.CurrentNode()
			if node.Type() == "method_declaration" || node.Type() == "constructor_declaration" {
				if elem, kind := getDefinitionElement(node, filePath); kind == model.Method {
					// 确保 QN 已在 Phase 1 中计算
					qn := resolveQualifiedName(node, filePath, gc)
					elem.QualifiedName = qn
					return elem
				}
			}
			if node.Type() == "class_declaration" || node.Type() == "interface_declaration" {
				// 如果找到类，停止查找方法
				break
			}
			if !cursor.GoToParent() {
				break
			}
		}
	}

	// 回退到 Class
	// ... 向上遍历查找最近的 Class ...

	// 回退到 File
	return &model.CodeElement{Kind: model.File, QualifiedName: filePath, Path: filePath}
}

// resolveQualifiedName 尝试使用 GlobalContext 解析 QN
func resolveQualifiedName(n *sitter.Node, filePath string, gc *model.GlobalContext) string {
	name := n.Content()

	// 1. 尝试在当前文件的上下文中查找（本地定义）
	if fc, ok := gc.FileContexts[filePath]; ok {
		if entry, ok := fc.Definitions[model.BuildQualifiedName(fc.PackageName, name)]; ok {
			return entry.Element.QualifiedName
		}
		// 尝试使用当前文件的 PackageName 作为前缀 (例如，本地类)
		if fc.PackageName != "" {
			possibleQN := fmt.Sprintf("%s.%s", fc.PackageName, name)
			if definitions := gc.ResolveQN(possibleQN); len(definitions) > 0 {
				return possibleQN
			}
		}
	}

	// 2. 尝试全局查找 (解决跨文件/包引用)
	if definitions := gc.ResolveQN(name); len(definitions) > 0 {
		return definitions[0].Element.QualifiedName // 找到直接 QN 匹配
	}

	// 3. 回退：假设它是一个导入的类或外部库 (这是最弱的解析)
	return name
}

// nodeToLocation 将 AST 节点转换为 Location 结构
func nodeToLocation(n *sitter.Node, filePath string) *model.Location {
	return &model.Location{
		FilePath:    filePath,
		StartLine:   int(n.StartPoint().Row) + 1,
		EndLine:     int(n.EndPoint().Row) + 1,
		StartColumn: int(n.StartPoint().Column),
		EndColumn:   int(n.EndPoint().Column),
	}
}

// getNodeContent 获取 AST 节点对应的源码文本内容
func getNodeContent(n *sitter.Node) string {
	if n == nil {
		return ""
	}
	// 使用 Bytes() 获取内容，并转换为 string
	return string(n.)
}