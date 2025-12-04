package java

import (
	"fmt"
	"strings"

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
	// JavaDefinitionQuery 收集定义和结构关系 (CONTAIN, EXTEND, IMPLEMENT, USE, ANNOTATION)
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
                   (field_declaration 
                      type: (_) @field_type
                      (variable_declarator (identifier) @field_name)
                   ) @field_def
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
          
          ; 3. 字段/变量读取 (USE) - 针对简单的字段访问，非方法调用
          (field_access field: (identifier) @use_field_name) @use_field_stmt

          ; 4. 显式类型转换 (CAST)
          (cast_expression type: (_) @cast_type) @cast_stmt
          
          ; 5. 通用标识符引用 (USE) - 捕获所有独立标识符，用于查找局部变量和未解析的类型
          (identifier) @use_identifier
          
          ; 6. 独立注解 (ANNOTATION) - 针对局部变量或方法体内的表达式
          (local_variable_declaration
             (modifiers (annotation name: (identifier) @annotation_name)) @annotation_stmt_local
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
	// 假设 model.LangJava 存在
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

type RelationHandler func(q *sitter.Query, match *sitter.QueryMatch, sourceBytes *[]byte, filePath string, gc *model.GlobalContext) ([]*model.DependencyRelation, error)

// handleDefinitionAndStructureRelations 处理 IMPORT, EXTEND, IMPLEMENT, TYPE USE, ANNOTATION
func (e *Extractor) handleDefinitionAndStructureRelations(q *sitter.Query, match *sitter.QueryMatch, sourceBytes *[]byte, filePath string, gc *model.GlobalContext) ([]*model.DependencyRelation, error) {
	relations := make([]*model.DependencyRelation, 0)

	// 找到最近的定义作为 Source，如果找不到则默认为 File
	sourceNode := match.NodesForCaptureIndex(0)[0]
	sourceElement := determineSourceElement(&sourceNode, sourceBytes, filePath, gc)
	if sourceElement == nil {
		sourceElement = &model.CodeElement{Kind: model.File, QualifiedName: filePath, Path: filePath}
	}

	// 1. IMPORT (已实现)
	if importTargetNode := findCapturedNode(q, match, sourceBytes, "import_target"); importTargetNode != nil {
		importName := getNodeContent(importTargetNode, *sourceBytes)
		relations = append(relations, &model.DependencyRelation{
			Type:     model.Import,
			Source:   sourceElement,
			Target:   &model.CodeElement{Kind: model.Package, Name: importName, QualifiedName: importName},
			Location: nodeToLocation(importTargetNode, filePath),
		})
	}

	// 2. EXTEND (Class)
	if extendsNode := findCapturedNode(q, match, sourceBytes, "extends_class"); extendsNode != nil {
		relations = append(relations, &model.DependencyRelation{
			Type:     model.Extend,
			Source:   sourceElement,
			Target:   &model.CodeElement{Kind: model.Class, Name: getNodeContent(extendsNode, *sourceBytes), QualifiedName: resolveQualifiedName(extendsNode, sourceBytes, filePath, gc)},
			Location: nodeToLocation(extendsNode, filePath),
		})
	}

	// 3. EXTEND (Interface)
	// 注意：interface_declaration 中的 extends_interface 也是继承关系
	if extendsNode := findCapturedNode(q, match, sourceBytes, "extends_interface"); extendsNode != nil {
		// 接口可以继承多个，所以这里可能需要迭代，但Query只返回一个匹配，我们只处理当前匹配到的
		relations = append(relations, &model.DependencyRelation{
			Type:     model.Extend,
			Source:   sourceElement,
			Target:   &model.CodeElement{Kind: model.Interface, Name: getNodeContent(extendsNode, *sourceBytes), QualifiedName: resolveQualifiedName(extendsNode, sourceBytes, filePath, gc)},
			Location: nodeToLocation(extendsNode, filePath),
		})
	}

	// 4. IMPLEMENT
	if implementsNode := findCapturedNode(q, match, sourceBytes, "implements_interface"); implementsNode != nil {
		// 接口可以实现多个，所以这里可能需要迭代
		relations = append(relations, &model.DependencyRelation{
			Type:     model.Implement,
			Source:   sourceElement,
			Target:   &model.CodeElement{Kind: model.Interface, Name: getNodeContent(implementsNode, *sourceBytes), QualifiedName: resolveQualifiedName(implementsNode, sourceBytes, filePath, gc)},
			Location: nodeToLocation(implementsNode, filePath),
		})
	}

	// 5. ANNOTATION (在 Class/Interface 级别)
	if annotationNameNode := findCapturedNode(q, match, sourceBytes, "annotation_name"); annotationNameNode != nil {
		// Annotation is attached to the class/interface itself (sourceElement)
		relations = append(relations, &model.DependencyRelation{
			Type:     model.Annotation,
			Source:   sourceElement,
			Target:   &model.CodeElement{Kind: model.Annotationn, Name: getNodeContent(annotationNameNode, *sourceBytes), QualifiedName: resolveQualifiedName(annotationNameNode, sourceBytes, filePath, gc)},
			Location: nodeToLocation(annotationNameNode, filePath),
		})
	}

	// 6. TYPE USAGE (Return Type, Parameter Type, Throws Type, Field Type)

	// 6a. Return Type
	if returnTypeNode := findCapturedNode(q, match, sourceBytes, "return_type"); returnTypeNode != nil {
		relations = append(relations, e.createTypeUsageRelation(sourceElement, returnTypeNode, sourceBytes, filePath, gc, "Return Type"))
	}

	// 6b. Parameter Type
	if paramTypeNode := findCapturedNode(q, match, sourceBytes, "param_type"); paramTypeNode != nil {
		relations = append(relations, e.createTypeUsageRelation(sourceElement, paramTypeNode, sourceBytes, filePath, gc, "Parameter Type"))
	}

	// 6c. Throws Type
	if throwsTypeNode := findCapturedNode(q, match, sourceBytes, "throws_type"); throwsTypeNode != nil {
		relations = append(relations, e.createTypeUsageRelation(sourceElement, throwsTypeNode, sourceBytes, filePath, gc, "Throws Type"))
	}

	// 6d. Field Type
	if fieldTypeNode := findCapturedNode(q, match, sourceBytes, "field_type"); fieldTypeNode != nil {
		relations = append(relations, e.createTypeUsageRelation(sourceElement, fieldTypeNode, sourceBytes, filePath, gc, "Field Type"))
	}

	return relations, nil
}

// createTypeUsageRelation 创建一个 USE 类型的依赖关系，目标类型为 Class/Interface
func (e *Extractor) createTypeUsageRelation(source *model.CodeElement, targetNode *sitter.Node, sourceBytes *[]byte, filePath string, gc *model.GlobalContext, detail string) *model.DependencyRelation {
	typeName := getNodeContent(targetNode, *sourceBytes)
	return &model.DependencyRelation{
		Type:     model.Use,
		Source:   source,
		Target:   &model.CodeElement{Kind: model.Type, Name: typeName, QualifiedName: resolveQualifiedName(targetNode, sourceBytes, filePath, gc)},
		Location: nodeToLocation(targetNode, filePath),
		Details:  detail,
	}
}

// handleActionRelations 处理 CALL, CREATE, USE, CAST, ANNOTATION
func (e *Extractor) handleActionRelations(q *sitter.Query, match *sitter.QueryMatch, sourceBytes *[]byte, filePath string, gc *model.GlobalContext) ([]*model.DependencyRelation, error) {
	relations := make([]*model.DependencyRelation, 0)

	sourceNode := match.NodesForCaptureIndex(0)[0]
	sourceElement := determineSourceElement(&sourceNode, sourceBytes, filePath, gc)
	if sourceElement == nil {
		sourceElement = &model.CodeElement{Kind: model.File, QualifiedName: filePath, Path: filePath}
	}

	// 1. CALL (已实现)
	if callTarget := findCapturedNode(q, match, sourceBytes, "call_target"); callTarget != nil {
		callStmt := callTarget.Parent()
		relations = append(relations, &model.DependencyRelation{
			Type:     model.Call,
			Source:   sourceElement,
			Target:   &model.CodeElement{Kind: model.Method, Name: getNodeContent(callTarget, *sourceBytes), QualifiedName: resolveQualifiedName(callTarget, sourceBytes, filePath, gc)},
			Location: nodeToLocation(callStmt, filePath),
			Details:  "Method Call",
		})
	}

	// 2. CREATE
	if createTarget := findCapturedNode(q, match, sourceBytes, "create_target_name"); createTarget != nil {
		createStmt := createTarget.Parent() // object_creation_expression
		relations = append(relations, &model.DependencyRelation{
			Type:     model.Create,
			Source:   sourceElement,
			Target:   &model.CodeElement{Kind: model.Class, Name: getNodeContent(createTarget, *sourceBytes), QualifiedName: resolveQualifiedName(createTarget, sourceBytes, filePath, gc)},
			Location: nodeToLocation(createStmt, filePath),
			Details:  "Object Creation",
		})
	}

	// 3. FIELD/VARIABLE USE
	if useFieldName := findCapturedNode(q, match, sourceBytes, "use_field_name"); useFieldName != nil {
		useStmt := useFieldName.Parent() // field_access
		relations = append(relations, &model.DependencyRelation{
			Type:     model.Use,
			Source:   sourceElement,
			Target:   &model.CodeElement{Kind: model.Field, Name: getNodeContent(useFieldName, *sourceBytes), QualifiedName: resolveQualifiedName(useFieldName, sourceBytes, filePath, gc)},
			Location: nodeToLocation(useStmt, filePath),
			Details:  "Field Access",
		})
	}

	// 4. CAST
	if castType := findCapturedNode(q, match, sourceBytes, "cast_type"); castType != nil {
		castStmt := castType.Parent() // cast_expression
		relations = append(relations, &model.DependencyRelation{
			Type:     model.Cast,
			Source:   sourceElement,
			Target:   &model.CodeElement{Kind: model.Type, Name: getNodeContent(castType, *sourceBytes), QualifiedName: resolveQualifiedName(castType, sourceBytes, filePath, gc)},
			Location: nodeToLocation(castStmt, filePath),
			Details:  "Explicit Type Cast",
		})
	}

	// 5. GENERIC IDENTIFIER USE
	// 注意：这会捕获很多东西（包括方法名、类名、变量名），需要避免重复和噪音。
	// 在实践中，通常只捕获那些没有被更具体关系（如 CALL, CREATE）捕获的标识符。
	// 为了简化，我们只将其作为通用类型/变量引用的Fallback。
	if genericID := findCapturedNode(q, match, sourceBytes, "use_identifier"); genericID != nil {
		// 排除方法调用和字段访问中的标识符，因为它们已经被 CALL/USE FIELD 捕获
		parentType := genericID.Parent().Kind()
		if parentType != "method_invocation" && parentType != "field_access" && genericID.Kind() == "identifier" {
			// 假设这是一个变量或未解析的类型引用
			relations = append(relations, &model.DependencyRelation{
				Type:     model.Use,
				Source:   sourceElement,
				Target:   &model.CodeElement{Kind: model.Unknown, Name: getNodeContent(genericID, *sourceBytes), QualifiedName: resolveQualifiedName(genericID, sourceBytes, filePath, gc)},
				Location: nodeToLocation(genericID, filePath),
				Details:  "Generic Identifier Use",
			})
		}
	}

	// 6. ANNOTATION (在局部或 Field 级别)
	if annotationNameNode := findCapturedNode(q, match, sourceBytes, "annotation_name"); annotationNameNode != nil {
		relations = append(relations, &model.DependencyRelation{
			Type:     model.Annotation,
			Source:   sourceElement,
			Target:   &model.CodeElement{Kind: model.Annotationn, Name: getNodeContent(annotationNameNode, *sourceBytes), QualifiedName: resolveQualifiedName(annotationNameNode, sourceBytes, filePath, gc)},
			Location: nodeToLocation(annotationNameNode, filePath),
			Details:  "Annotation Usage",
		})
	}

	return relations, nil
}

// --- 通用辅助函数实现 (用于上下文完整性) ---

// processQuery 运行 Tree-sitter 查询并处理匹配项
func (e *Extractor) processQuery(rootNode *sitter.Node, sourceBytes *[]byte, tsLang *sitter.Language, queryStr string, filePath string, gc *model.GlobalContext, relations *[]*model.DependencyRelation, handler RelationHandler) error {
	formatQueryStr := strings.ReplaceAll(queryStr, "\t", " ")
	formatQueryStr = strings.ReplaceAll(queryStr, "\n", " ")

	q, err := sitter.NewQuery(tsLang, formatQueryStr)
	if err != nil {
		return fmt.Errorf("failed to create query: %w", err)
	}
	defer q.Close()

	qc := sitter.NewQueryCursor()
	matches := qc.Matches(q, rootNode, *sourceBytes)

	for {
		match := matches.Next()
		if match == nil {
			break
		}

		newRelations, err := handler(q, match, sourceBytes, filePath, gc)
		if err != nil {
			return err
		}
		*relations = append(*relations, newRelations...)
	}

	return nil
}

// findCapturedNode 从匹配中查找指定名称的捕获节点
func findCapturedNode(q *sitter.Query, match *sitter.QueryMatch, sourceBytes *[]byte, name string) *sitter.Node {
	index, ok := q.CaptureIndexForName(name)
	if !ok {
		return nil
	}

	nodes := match.NodesForCaptureIndex(index)
	if len(nodes) > 0 {
		return &nodes[0]
	}
	return nil
}

// determineSourceElement 向上遍历 AST 查找最近的 Method/Class 作为关系 Source
func determineSourceElement(n *sitter.Node, sourceBytes *[]byte, filePath string, gc *model.GlobalContext) *model.CodeElement {
	cursor := n.Walk()
	defer cursor.Close()

	if cursor.GotoParent() {
		for {
			node := cursor.Node()
			nodeType := node.Kind()

			if nodeType == "method_declaration" || nodeType == "constructor_declaration" {
				if elem, kind := getDefinitionElement(node, sourceBytes, filePath); kind == model.Method {
					qn := resolveQualifiedName(node, sourceBytes, filePath, gc)
					elem.QualifiedName = qn
					return elem
				}
			}
			if nodeType == "class_declaration" || nodeType == "interface_declaration" {
				if elem, kind := getDefinitionElement(node, sourceBytes, filePath); kind == model.Class || kind == model.Interface {
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
func resolveQualifiedName(n *sitter.Node, sourceBytes *[]byte, filePath string, gc *model.GlobalContext) string {
	name := getNodeContent(n, *sourceBytes)

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

// nodeToLocation
func nodeToLocation(n *sitter.Node, filePath string) *model.Location {
	return &model.Location{
		FilePath:    filePath,
		StartLine:   int(n.StartPosition().Row) + 1,
		EndLine:     int(n.EndPosition().Row) + 1,
		StartColumn: int(n.StartPosition().Column),
		EndColumn:   int(n.EndPosition().Column),
	}
}
