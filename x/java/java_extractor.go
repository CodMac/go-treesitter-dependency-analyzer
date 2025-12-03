package java

import (
	"fmt"
	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	"github.com/CodMac/go-treesitter-dependency-analyzer/parser"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Extractor 实现了 extractor.Extractor 接口
type Extractor struct{}

func NewJavaExtractor() *Extractor {
	return &Extractor{}
}

// --- Tree-sitter Queries ---

const (
	// JavaDefinitionQuery 收集定义和结构关系 (CONTAIN, EXTEND, IMPLEMENT)
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
	// JavaRelationQuery 收集操作关系 (CALL, CREATE, USE, CAST, ANNOTATION)
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

// Extract 实现了 extractor.ContextExtractor 接口
func (e *Extractor) Extract(rootNode *sitter.Node, filePath string, gCtx *model.GlobalContext) ([]*model.DependencyRelation, error) {
	fCtx, ok := gCtx.FileContexts[filePath]
	if !ok {
		return nil, fmt.Errorf("failed to get FileContext: %s", filePath)
	}

	relations := make([]*model.DependencyRelation, 0)
	tsLang, _ := parser.GetLanguage(model.LangJava)
	sourceBytes := fCtx.SourceBytes

	// 1. 结构和定义关系
	if err := e.processQuery(rootNode, sourceBytes, tsLang, JavaDefinitionQuery, filePath, gCtx, &relations, e.handleDefinitionAndStructureRelations); err != nil {
		return nil, fmt.Errorf("failed to process definition query: %w", err)
	}

	// 2. 操作关系
	if err := e.processQuery(rootNode, sourceBytes, tsLang, JavaRelationQuery, filePath, gCtx, &relations, e.handleActionRelations); err != nil {
		return nil, fmt.Errorf("failed to process relation query: %w", err)
	}

	return relations, nil
}

type RelationHandler func(q *sitter.Query, match *sitter.QueryMatch, sourceBytes []byte, filePath string, gc *model.GlobalContext) ([]*model.DependencyRelation, error)

// handleDefinitionAndStructureRelations
func (e *Extractor) handleDefinitionAndStructureRelations(q *sitter.Query, match *sitter.QueryMatch, sourceBytes *[]byte, filePath string, gc *model.GlobalContext) ([]*model.DependencyRelation, error) {
	relations := make([]*model.DependencyRelation, 0)

	// 由于 QueryMatches.Next() 只返回 *QueryMatch，我们无法像以前那样轻松获取根节点。
	// 简单起见，我们假设匹配中的第一个捕获是主节点。
	sourceNode := match.NodesForCaptureIndex(0)[0]
	sourceElement := determineSourceElement(sourceNode, sourceBytes, filePath, gc)
	if sourceElement == nil {
		sourceElement = &model.CodeElement{Kind: model.File, QualifiedName: filePath, Path: filePath}
	}

	// 1. IMPORT
	if importTargetNode := findCapturedNode(q, match, sourceBytes, "import_target"); importTargetNode != nil {
		importName := getNodeContent(importTargetNode, *sourceBytes)
		relations = append(relations, &model.DependencyRelation{
			Type:     model.Import,
			Source:   sourceElement,
			Target:   &model.CodeElement{Kind: model.Package, Name: importName, QualifiedName: importName},
			Location: nodeToLocation(importTargetNode, filePath),
		})
	}

	// 2. EXTEND
	if extendsNode := findCapturedNode(q, match, sourceBytes, "extends_class"); extendsNode != nil {
		relations = append(relations, &model.DependencyRelation{
			Type:     model.Extend,
			Source:   sourceElement,
			Target:   &model.CodeElement{Kind: model.Class, Name: getNodeContent(extendsNode, *sourceBytes), QualifiedName: resolveQualifiedName(extendsNode, sourceBytes, filePath, gc)},
			Location: nodeToLocation(extendsNode, filePath),
		})
	}

	// ... (其他关系处理逻辑类似，都需要传入 sourceBytes 并使用 getNodeContent 和 resolveQualifiedName 的新签名) ...

	return relations, nil
}

// handleActionRelations
func (e *Extractor) handleActionRelations(q *sitter.Query, match *sitter.QueryMatch, sourceBytes *[]byte, filePath string, gc *model.GlobalContext) ([]*model.DependencyRelation, error) {
	relations := make([]*model.DependencyRelation, 0)

	sourceNode := match.NodesForCaptureIndex(0)[0]
	sourceElement := determineSourceElement(sourceNode, sourceBytes, filePath, gc)
	if sourceElement == nil {
		sourceElement = &model.CodeElement{Kind: model.File, QualifiedName: filePath, Path: filePath}
	}

	// 1. CALL
	if callTarget := findCapturedNode(q, match, sourceBytes, "call_target"); callTarget != nil {
		callStmt := callTarget.Parent()
		relations = append(relations, &model.DependencyRelation{
			Type:     model.Call,
			Source:   sourceElement,
			Target:   &model.CodeElement{Kind: model.Method, Name: getNodeContent(callTarget, *sourceBytes), QualifiedName: resolveQualifiedName(callTarget, sourceBytes, filePath, gc)},
			Location: nodeToLocation(callStmt, filePath),
		})
	}

	// ... (其他关系处理逻辑类似) ...

	return relations, nil
}

// --- 通用辅助函数实现 (V0.24.0 兼容) ---

// processQuery 运行 Tree-sitter 查询并处理匹配项
// FIX: 使用 qc.Matches(query, node, text)
func (e *Extractor) processQuery(rootNode *sitter.Node, sourceBytes *[]byte, tsLang *sitter.Language, queryStr string, filePath string, gc *model.GlobalContext, relations *[]*model.DependencyRelation, handler RelationHandler) error {
	q, err := sitter.NewQuery(tsLang, queryStr)
	if err != nil {
		return fmt.Errorf("failed to create query: %w", err)
	}
	defer q.Close()

	qc := sitter.NewQueryCursor()
	// FIX: 使用 Matches()，它返回 QueryMatches 迭代器
	matches := qc.Matches(q, rootNode, *sourceBytes)

	for {
		match := matches.Next()
		if match == nil {
			break
		}

		newRelations, err := handler(q, match, *sourceBytes, filePath, gc)
		if err != nil {
			return err
		}
		*relations = append(*relations, newRelations...)
	}

	return nil
}

// findCapturedNode 从匹配中查找指定名称的捕获节点
// FIX: 使用 q.CaptureNames() 查找索引，然后使用 NodesForCaptureIndex 获取节点
func findCapturedNode(q *sitter.Query, match *sitter.QueryMatch, sourceBytes *[]byte, name string) *sitter.Node {
	// FIX: 使用 CaptureIndexForName 查找索引
	index, ok := q.CaptureIndexForName(name)
	if !ok {
		return nil
	}

	// FIX: 使用 NodesForCaptureIndex 获取节点
	nodes := match.NodesForCaptureIndex(index)
	if len(nodes) > 0 {
		return nodes[0]
	}
	return nil
}

// determineSourceElement 向上遍历 AST 查找最近的 Method/Class 作为关系 Source
// FIX: 签名需要 sourceBytes
func determineSourceElement(n *sitter.Node, sourceBytes *[]byte, filePath string, gc *model.GlobalContext) *model.CodeElement {
	// FIX: 使用 n.Walk()
	cursor := n.Walk()
	defer cursor.Close()

	// FIX: 使用 cursor.GotoParent()
	if cursor.GotoParent() {
		for {
			// FIX: 使用 cursor.Node()
			node := cursor.Node()
			// FIX: 使用 node.Kind()
			nodeType := node.Kind()

			if nodeType == "method_declaration" || nodeType == "constructor_declaration" {
				if elem, kind := getDefinitionElement(node, sourceBytes, filePath); kind == model.Method {
					// FIX: 传递 sourceBytes
					qn := resolveQualifiedName(node, sourceBytes, filePath, gc)
					elem.QualifiedName = qn
					return elem
				}
			}
			if nodeType == "class_declaration" || nodeType == "interface_declaration" {
				if elem, kind := getDefinitionElement(node, sourceBytes, filePath); kind == model.Class || kind == model.Interface {
					// FIX: 传递 sourceBytes
					qn := resolveQualifiedName(node, sourceBytes, filePath, gc)
					elem.QualifiedName = qn
					return elem
				}
				break
			}
			if !cursor.GotoParent() {
				break
			}
		}
	}

	return &model.CodeElement{Kind: model.File, QualifiedName: filePath, Path: filePath}
}

// resolveQualifiedName 尝试使用 GlobalContext 解析 QN
// FIX: 签名需要 sourceBytes
func resolveQualifiedName(n *sitter.Node, sourceBytes *[]byte, filePath string, gc *model.GlobalContext) string {
	name := getNodeContent(n, *sourceBytes) // FIX: 传递 sourceBytes

	// ... (解析逻辑不变) ...

	if fc, ok := gc.FileContexts[filePath]; ok {
		if entry, ok := fc.Definitions[name]; ok {
			return entry.Element.QualifiedName
		}

		if fc.PackageName != "" {
			possibleQN := model.BuildQualifiedName(fc.PackageName, name)
			if definitions := gc.ResolveQN(possibleQN); len(definitions) > 0 {
				return possibleQN
			}
		}
	}

	if definitions := gc.ResolveQN(name); len(definitions) > 0 {
		return definitions[0].Element.QualifiedName
	}

	return name
}

// nodeToLocation 保持不变，因为 StartPoint() 和 EndPoint() 存在
func nodeToLocation(n *sitter.Node, filePath string) *model.Location {
	return &model.Location{
		FilePath:    filePath,
		StartLine:   int(n.StartPosition().Row) + 1,
		EndLine:     int(n.EndPosition().Row) + 1,
		StartColumn: int(n.StartPosition().Column),
		EndColumn:   int(n.EndPosition().Column),
	}
}
