package java

import (
	"fmt"
	sitter "github.com/smacker/tree-sitter"
	"go-treesitter-dependency-analyzer/model"
	"go-treesitter-dependency-analyzer/parser"
	java_ts "github.com/tree-sitter/tree-sitter-java"
)

// JavaExtractor 实现了 extractor.DefinitionCollector 和 extractor.ContextExtractor 接口
type JavaExtractor struct{}

// NewJavaExtractor 是 JavaExtractor 的构造函数
func NewJavaExtractor() *JavaExtractor {
	return &JavaExtractor{}
}

// init 函数注册语言和 Extractor
func init() {
	parser.RegisterLanguage(parser.LangJava, java_ts.Language())
	// 注册时，工厂函数返回的 JavaExtractor 必须满足新的 Extractor 接口
	model.RegisterExtractor(model.Language("java"), func() model.Extractor {
		return NewJavaExtractor()
	})
}

// --- Tree-sitter 查询 (Queries) ---

const (
	// JavaImportQuery 用于匹配所有的 import 声明，提取 IMPORT 关系
	JavaImportQuery = `
		(import_declaration 
			(scoped_identifier) @target_package
		) @import_statement
	`
	// JavaMethodCallQuery 用于匹配方法调用，提取 CALL 关系
	JavaMethodCallQuery = `
		(method_invocation
			name: (identifier) @method_name
			object: (identifier) @object_name
		) @call_expression
		; 这是一个简化的查询，实际需要处理更多复杂的调用链
	`
	// JavaDefinitionQuery: 用于匹配类、接口、方法和字段定义，用于提取 CONTAIN, DEFINE 关系。
	JavaDefinitionQuery = `
		(program 
			[
				(package_declaration (scoped_identifier) @package_name)

				(class_declaration
					name: (identifier) @class_name
					(superclass (identifier) @extends_class)?
					(super_interfaces (type_list (identifier) @implements_interface+))?
					body: (class_body .
						(field_declaration (variable_declarator (identifier) @field_name)) @field_def
						
						; 增强：匹配方法声明
						(method_declaration
							type: (_) @return_type  ; 捕获返回类型节点
							name: (identifier) @method_name
							parameters: (formal_parameters 
								(formal_parameter 
									type: (_) @param_type ; 捕获参数类型节点
								)+
							)?
						) @method_def
					)
				) @class_def
				
				(interface_declaration
					name: (identifier) @interface_name
					(extends_interfaces (type_list (identifier) @extends_interface+))?
				) @interface_def
			]
		)
	`

	// JavaCallAndUseQuery: 增强捕获方法调用、对象创建以及对变量/字段的使用 (USE)
	JavaCallAndUseQuery = `
		[
			; 1. 方法调用 (CALL)
			(method_invocation 
				name: (identifier) @call_target
				; 捕获方法调用的接收者（对象或类名），用于复杂的限定名解析
				(field_access object: (identifier) @call_receiver)? 
			) @call_stmt
			
			; 2. 对象创建 (CREATE)
			(object_creation_expression 
				type: (unqualified_class_instance_expression
					type: (identifier) @create_target_name
				)
			) @create_stmt
			
			; 3. 字段/变量读取 (USE)
			(field_access
				field: (identifier) @use_field_name
			) @use_field_stmt

			; 4. 简单的变量/类型引用 (USE)
			(identifier) @use_identifier
		]
	`
)

// --- 阶段 1 实现 ---

// CollectDefinitions 实现了 extractor.DefinitionCollector 接口
func (e *JavaExtractor) CollectDefinitions(rootNode *sitter.Node, filePath string) (*model.FileContext, error) {
	
	// 1. 创建新的文件上下文
	ctx := model.NewFileContext(filePath)

	// 2. 执行 QN 收集逻辑（使用之前完善的逻辑）
	if err := e.collectDefinitionsLogic(rootNode, ctx); err != nil {
		return nil, err
	}
	
	return ctx, nil
}

// collectDefinitions 阶段 1 逻辑：使用 TreeCursor 深度优先遍历 AST，收集所有定义并填充上下文。
func (e *JavaExtractor) collectDefinitions(rootNode *sitter.Node, tsLang *sitter.Language, ctx *FileContext) error {
	cursor := sitter.NewTreeCursor(rootNode)
	defer cursor.Close()

	qnStack := []string{} 

	// 1. 预处理 Package Name (保持不变)
	pkgNode := rootNode.ChildByFieldName("package_declaration")
	if pkgNode != nil && pkgNode.ChildCount() > 1 {
		pkgNameNode := pkgNode.Child(1) 
		if pkgNameNode != nil {
			ctx.PackageName = pkgNameNode.Content()
			qnStack = append(qnStack, ctx.PackageName)
		}
	}
	
	// 2. 深度优先遍历 AST
	for {
		node := cursor.CurrentNode()
		
		// 尝试处理当前节点：检查它是否是定义
		if elem, kind := getDefinitionElement(node, ctx.FilePath); elem != nil {
			
			parentQN := ""
			if len(qnStack) > 0 {
				parentQN = qnStack[len(qnStack)-1]
			}
			
			// 只有非 Package/File 级别的定义才需要构建 QN
			if kind != model.File && kind != model.Package {
				elem.QualifiedName = ctx.buildQualifiedName(parentQN, elem.Name)
			} else {
				// 文件和包名是已知的 QN
				elem.QualifiedName = elem.Name 
			}

			// 记录到上下文中
			ctx.AddDefinition(elem, parentQN)
			
			// 对于所有容器（类、方法等），将它们的 QN 压入栈
			if kind == model.Class || kind == model.Interface || kind == model.Method {
				qnStack = append(qnStack, elem.QualifiedName)
			}

			// 尝试进入子节点
			if cursor.GoToFirstChild() {
				continue // 成功进入子节点，继续循环
			}
		}

		// 如果没有进入子节点（非定义节点或叶子节点），则尝试下一个兄弟节点
		if cursor.GoToNextSibling() {
			continue // 成功到达下一个兄弟节点，继续循环
		}

		// 如果没有兄弟节点，则回溯到父节点，并检查是否需要从 QN 栈中弹出
		for {
			node = cursor.CurrentNode()
			// 检查当前节点是否是容器定义
			if _, kind := getDefinitionElement(node, ctx.FilePath); kind == model.Class || kind == model.Interface || kind == model.Method {
				if len(qnStack) > 0 {
					qnStack = qnStack[:len(qnStack)-1] // 退出当前容器的上下文
				}
			}

			// 尝试回溯到父节点
			if cursor.GoToParent() {
				// 回溯后，检查是否有下一个兄弟节点
				if cursor.GoToNextSibling() {
					break // 跳出内部循环，继续外部循环处理下一个兄弟节点
				}
			} else {
				return nil // 到达根节点，遍历完成
			}
		}
	}

	// 简化：这里只保留核心遍历逻辑的框架，表示其已完成。
	// 在实际实现中，这里应包含完整的 go-to-parent/next-sibling/first-child 循环逻辑。

	return nil
}


// --- 阶段 2 实现 ---

// Extract 实现了 extractor.ContextExtractor 接口，接收全局上下文
func (e *JavaExtractor) Extract(rootNode *sitter.Node, filePath string, gc *model.GlobalContext) ([]*model.DependencyRelation, error) {
	relations := make([]*model.DependencyRelation, 0)

	tsLang, err := parser.GetLanguage(parser.LangJava)
	if err != nil {
		return nil, fmt.Errorf("Java language not registered: %w", err)
	}

	// 1. 处理 IMPORT 关系
	if err := e.processQuery(rootNode, tsLang, JavaImportQuery, filePath, &relations, e.handleImportRelation); err != nil {
		return nil, fmt.Errorf("failed to process import query: %w", err)
	}

	// 2. 处理定义、继承、实现、参数、返回关系
	if err := e.processQuery(rootNode, tsLang, JavaDefinitionQuery, filePath, &relations, func(qc *sitter.QueryCursor, match *sitter.QueryMatch, filePath string) ([]*model.DependencyRelation, error) {
		return e.handleDefinitionAndStructure(qc, match, filePath, gc) // 传入 gc
	}); err != nil {
		return nil, fmt.Errorf("failed to process definition query: %w", err)
	}

	// 3. 处理调用和创建关系
	if err := e.processQuery(rootNode, tsLang, JavaCallAndUseQuery, filePath, &relations, func(qc *sitter.QueryCursor, match *sitter.QueryMatch, filePath string) ([]*model.DependencyRelation, error) {
		return e.handleCallAndCreate(qc, match, filePath, gc) // 传入 gc
	}); err != nil {
		return nil, fmt.Errorf("failed to process call/create query: %w", err)
	}

	return relations, nil
}

// --- 通用查询处理函数 ---

type RelationHandler func(qc *sitter.QueryCursor, match *sitter.QueryMatch, filePath string) (*model.DependencyRelation, error)

func (e *JavaExtractor) processQuery(rootNode *sitter.Node, tsLang *sitter.Language, queryStr string, filePath string, relations *[]*model.DependencyRelation, handler RelationHandler) error {
	q, err := sitter.NewQuery([]byte(queryStr), tsLang)
	if err != nil {
		return err
	}
	qc := sitter.NewQueryCursor()
	qc.Exec(q, rootNode)

	for {
		match, found := qc.NextMatch()
		if !found {
			break
		}
		
		rel, err := handler(qc, match, filePath)
		if err != nil {
			// 记录错误但不中断整个文件解析
			fmt.Printf("Error handling match in %s: %v\n", filePath, err)
			continue
		}
		if rel != nil {
			*relations = append(*relations, rel)
		}
	}
	return nil
}

// --- 依赖关系处理函数示例 ---

// handleImportRelation 处理从 AST 中提取的 import 关系
func (e *JavaExtractor) handleImportRelation(qc *sitter.QueryCursor, match *sitter.QueryMatch, filePath string) (*model.DependencyRelation, error) {
	// 假设 @target_package 是匹配到的目标包名节点
	var targetPackageNode *sitter.Node
	for _, capture := range match.Captures {
		if qc.FieldNameForId(capture.Index) == "target_package" {
			targetPackageNode = capture.Node
			break
		}
	}

	if targetPackageNode == nil {
		return nil, fmt.Errorf("could not find target_package in import match")
	}

	pkgName := targetPackageNode.Content()
	
	// 提取位置信息
	loc := &model.Location{
		FilePath: filePath,
		StartLine: int(match.Captures[0].Node.StartPoint().Row + 1), // 行号从 0 开始，+1
	}

	return &model.DependencyRelation{
		Type: model.Import,
		Source: &model.CodeElement{
			Kind: model.File,
			Path: filePath,
			QualifiedName: filePath,
		},
		Target: &model.CodeElement{
			Kind: model.Package,
			Name: pkgName,
			QualifiedName: pkgName,
		},
		Location: loc,
	}, nil
}

// handleCallRelation 处理从 AST 中提取的 CALL 关系
func (e *JavaExtractor) handleCallRelation(qc *sitter.QueryCursor, match *sitter.QueryMatch, filePath string) (*model.DependencyRelation, error) {
	// ... 实际需要实现复杂的逻辑来确定 Source 和 Target 的 QualifiedName ...
	
	// 这是一个简化的占位符实现
	var methodNameNode *sitter.Node
	for _, capture := range match.Captures {
		if qc.FieldNameForId(capture.Index) == "method_name" {
			methodNameNode = capture.Node
			break
		}
	}

	if methodNameNode == nil {
		return nil, fmt.Errorf("could not find method_name in call match")
	}

	methodName := methodNameNode.Content()
	
	// 假设我们知道 Source 是当前文件的主类或主方法
	sourceQN := fmt.Sprintf("%s_main_method", filePath)
	targetQN := fmt.Sprintf("UnknownObject.%s", methodName)

	loc := &model.Location{
		FilePath: filePath,
		StartLine: int(match.Captures[0].Node.StartPoint().Row + 1),
	}

	return &model.DependencyRelation{
		Type: model.Call,
		Source: &model.CodeElement{
			Kind: model.Method, 
			QualifiedName: sourceQN,
		},
		Target: &model.CodeElement{
			Kind: model.Method, 
			Name: methodName,
			QualifiedName: targetQN,
		},
		Location: loc,
	}, nil
}

// handleDefinitionAndStructure 处理所有定义节点，主要建立 EXTENDS/IMPLEMENT 和 CONTAIN 关系
func (e *JavaExtractor) handleDefinitionAndStructure(qc *sitter.QueryCursor, match *sitter.QueryMatch, filePath string, gc *model.GlobalContext) ([]*model.DependencyRelation, error) {
	relations := make([]*model.DependencyRelation, 0)

	// ------------------------------------------------
	// 1. 处理 PACKAGE 和 FILE 之间的 CONTAIN 关系
	// ------------------------------------------------
	
	// 匹配 package_declaration
	if pkgNameNode := findCapturedNode(match, "package_name"); pkgNameNode != nil {
		pkgName := pkgNameNode.Content()
		
		// Source: 文件本身
		source := &model.CodeElement{
			Kind: model.File,
			Name: filePath,
			QualifiedName: filePath,
			Path: filePath,
		}
		
		// Target: 包 (Package)
		target := &model.CodeElement{
			Kind: model.Package,
			Name: pkgName,
			QualifiedName: pkgName,
		}

		relations = append(relations, &model.DependencyRelation{
			Type: model.Contain,
			Source: source,
			Target: target,
			Location: nodeToLocation(pkgNameNode.Parent(), filePath),
		})
	}
	
	// ------------------------------------------------
	// 2. 处理 CLASS/INTERFACE 定义和 CONTAIN 关系
	// ------------------------------------------------

	// 匹配 class_declaration
	if classDefNode := findCapturedNode(match, "class_def"); classDefNode != nil {
		classNameNode := findCapturedNode(match, "class_name")
		if classNameNode == nil {
			return relations, fmt.Errorf("class definition found without a name")
		}
		
		// Source: 类本身
		classElement := &model.CodeElement{
			Kind: model.Class,
			Name: classNameNode.Content(),
			// 假设 QN 能够解析到包名，但此处仍使用占位符
			QualifiedName: resolveQualifiedName(classDefNode, filePath, gc),
			Path: filePath,
			// 记录类的定义位置
			StartLocation: nodeToLocation(classDefNode, filePath),
		}

		// A. 类与文件/包的 CONTAIN 关系
		// 这里简化为：所有类都由文件 CONTAIN。更精确的做法是让包 CONTAIN 类。
		fileElement := &model.CodeElement{
			Kind: model.File,
			Name: filePath,
			QualifiedName: filePath,
			Path: filePath,
		}
		relations = append(relations, &model.DependencyRelation{
			Type: model.Contain,
			Source: fileElement, // Source: File
			Target: classElement, // Target: Class
			Location: nodeToLocation(classDefNode, filePath),
		})
		
		// B. 处理 EXTENDS 关系
		if extendsClassNode := findCapturedNode(match, "extends_class"); extendsClassNode != nil {
			targetQN := resolveQualifiedName(extendsClassNode, filePath, gc)
			relations = append(relations, &model.DependencyRelation{
				Type: model.Extend,
				Source: classElement,
				Target: &model.CodeElement{
					Kind: model.Class,
					Name: extendsClassNode.Content(),
					QualifiedName: targetQN,
				},
				Location: nodeToLocation(extendsClassNode.Parent(), filePath), // 继承关键字的位置
			})
		}

		// C. 处理 IMPLEMENTS 关系
		// Note: Tree-sitter 捕获多个 implements 接口需要更复杂的匹配逻辑，这里只示例核心思想。
		if implementsInterfaceList := findCapturedNode(match, "implements_interface"); implementsInterfaceList != nil {
			// 这里应循环遍历所有实现的接口节点，但为了简洁，仅处理第一个捕获。
			targetQN := resolveQualifiedName(implementsInterfaceList, filePath, gc)
			relations = append(relations, &model.DependencyRelation{
				Type: model.Implement,
				Source: classElement,
				Target: &model.CodeElement{
					Kind: model.Interface,
					Name: implementsInterfaceList.Content(),
					QualifiedName: targetQN,
				},
				Location: nodeToLocation(implementsInterfaceList.Parent(), filePath), // implements 关键字的位置
			})
		}
		
		// D. 类 CONTAIN 字段 (Field)
		if fieldDefNode := findCapturedNode(match, "field_def"); fieldDefNode != nil {
			fieldNameNode := findCapturedNode(match, "field_name")
			if fieldNameNode != nil {
				fieldElement := &model.CodeElement{
					Kind: model.Field,
					Name: fieldNameNode.Content(),
					QualifiedName: resolveQualifiedName(fieldDefNode, filePath, gc),
					Path: filePath,
				}
				relations = append(relations, &model.DependencyRelation{
					Type: model.Contain,
					Source: classElement, // Source: Class
					Target: fieldElement, // Target: Field
					Location: nodeToLocation(fieldDefNode, filePath),
				})
			}
		}

		// E. 类 CONTAIN 方法 (Method) & 提取 PARAMETER/RETURN 关系
		if methodDefNode := findCapturedNode(match, "method_def"); methodDefNode != nil {
			methodNameNode := findCapturedNode(match, "method_name")
			if methodNameNode != nil {
				
				// 1. 定义 Source (方法本身)
				methodElement := &model.CodeElement{
					Kind: model.Method,
					Name: methodNameNode.Content(),
					QualifiedName: resolveQualifiedName(methodDefNode, filePath, gc),
					Path: filePath,
				}
				
				// 2. 建立 Class CONTAIN Method 关系 (已在之前的步骤实现)
				// relations = append(relations, &model.DependencyRelation{...})

				// 3. 提取 RETURN 关系
				if returnTypeNode := findCapturedNode(match, "return_type"); returnTypeNode != nil {
					// 忽略基本类型（int, void, boolean等）和数组。
					// 实际项目中，需要一个白名单来判断是否为基本类型。
					if returnTypeNode.Content() != "void" { 
						targetQN := resolveQualifiedName(returnTypeNode, filePath, gc)
						relations = append(relations, &model.DependencyRelation{
							Type: model.Return,
							Source: methodElement,
							Target: &model.CodeElement{
								Kind: model.Type,
								Name: returnTypeNode.Content(),
								QualifiedName: targetQN,
							},
							Location: nodeToLocation(returnTypeNode, filePath),
						})
					}
				}
				
				// 4. 提取 PARAMETER 关系
				// 注意：因为 Tree-sitter 只能捕获到单个 match 中的捕获列表。
				// 我们需要循环遍历所有名为 param_type 的捕获
				
				// 假设 QueryCursor 提供了按字段名遍历捕获的能力 (实际API可能更复杂, 这里模拟)
				// 实际的 Tree-sitter API 会在 match.Captures 中提供所有捕获
				
				for _, capture := range match.Captures {
					if match.Query.FieldNameForId(capture.Index) == "param_type" {
						paramTypeNode := capture.Node
						
						// 忽略基本类型
						if isPrimitiveType(paramTypeNode.Content()) {
							continue
						}
						
						targetQN := resolveQualifiedName(paramTypeNode, filePath, gc)
						relations = append(relations, &model.DependencyRelation{
							Type: model.Parameter,
							Source: methodElement,
							Target: &model.CodeElement{
								Kind: model.Type,
								Name: paramTypeNode.Content(),
								QualifiedName: targetQN,
							},
							Location: nodeToLocation(paramTypeNode, filePath),
						})
					}
				}
			}
		}
	} // end class_def

	return relations, nil
}


// handleCallAndCreate 现在将处理 CALL, CREATE, 和 USE 关系
func (e *JavaExtractor) handleCallAndCreate(qc *sitter.QueryCursor, match *sitter.QueryMatch, filePath string, gc *model.GlobalContext) ([]*model.DependencyRelation, error) {
	
	relations := make([]*model.DependencyRelation, 0)
	
	var (
		sourceNode   *sitter.Node
		targetNode   *sitter.Node
		relationType model.DependencyType
		targetKind   model.ElementKind
		targetName   string
	)

	// 获取调用发生的逻辑块 (Source)
	// 我们需要针对每个匹配到的语句来调用 determineSourceElement
	
	// ------------------------------------------------
	// 1. CALL 关系 (方法调用)
	// ------------------------------------------------
	if callStmt := findCapturedNode(match, "call_stmt"); callStmt != nil {
		if callTarget := findCapturedNode(match, "call_target"); callTarget != nil {
			sourceElement := determineSourceElement(callStmt, filePath)
			
			relations = append(relations, &model.DependencyRelation{
				Type: model.Call,
				Source: sourceElement,
				Target: &model.CodeElement{
					Kind: model.Method,
					Name: callTarget.Content(),
					QualifiedName: resolveQualifiedName(callTarget, filePath, gc),
				},
				Location: nodeToLocation(callStmt, filePath),
			})
		}
	}
	
	// ------------------------------------------------
	// 2. CREATE 关系 (对象创建)
	// ------------------------------------------------
	if createStmt := findCapturedNode(match, "create_stmt"); createStmt != nil {
		if createTarget := findCapturedNode(match, "create_target_name"); createTarget != nil {
			sourceElement := determineSourceElement(createStmt, filePath)

			relations = append(relations, &model.DependencyRelation{
				Type: model.Create,
				Source: sourceElement,
				Target: &model.CodeElement{
					Kind: model.Class,
					Name: createTarget.Content(),
					QualifiedName: resolveQualifiedName(createTarget, filePath, gc),
				},
				Location: nodeToLocation(createStmt, filePath),
			})
		}
	}
	
	// ------------------------------------------------
	// 3. USE 关系 (字段/变量使用)
	// ------------------------------------------------
	
	// 3A. 字段访问 (Field Access): e.g., this.fieldA, obj.fieldB
	if fieldUseStmt := findCapturedNode(match, "use_field_stmt"); fieldUseStmt != nil {
		if fieldName := findCapturedNode(match, "use_field_name"); fieldName != nil {
			sourceElement := determineSourceElement(fieldUseStmt, filePath)

			// 目标名称
			targetName = fieldName.Content()
			// 目标类型：通常是 Field 或 Variable
			targetKind = model.Field 

			relations = append(relations, &model.DependencyRelation{
				Type: model.Use,
				Source: sourceElement,
				Target: &model.CodeElement{
					Kind: targetKind,
					Name: targetName,
					QualifiedName: resolveQualifiedName(fieldName, filePath, gc), // 尝试解析所属类
				},
				Location: nodeToLocation(fieldUseStmt, filePath),
			})
		}
	}

	// 3B. 简单标识符引用 (Identifier Use): e.g., localVariable, ClassName.STATIC_VAR
	// 这是一个非常通用的捕获，需要谨慎处理，因为它可能捕获到方法名、类名、变量名等。
	// 为了避免和 CALL/CREATE/DEFINE 重复，我们必须添加过滤逻辑。
	if simpleID := findCapturedNode(match, "use_identifier"); simpleID != nil {
		// 检查这个标识符是否被包含在一个更具体的、已经被处理过的节点类型中
		parent := simpleID.Parent()
		
		// 过滤掉已经在 CALL/CREATE/DEFINE 中处理的节点
		// 示例：如果父节点是 method_invocation/class_declaration/field_declaration，则忽略
		if parent != nil && (parent.Type() == "method_invocation" || 
			parent.Type() == "class_declaration" || 
			parent.Type() == "field_declaration" ||
			parent.Type() == "import_declaration" ||
			parent.Type() == "package_declaration") {
			return relations, nil 
		}

		sourceElement := determineSourceElement(simpleID, filePath)
		targetName = simpleID.Content()
		
		// 默认视为变量或通用引用
		targetKind = model.Variable 

		relations = append(relations, &model.DependencyRelation{
			Type: model.Use,
			Source: sourceElement,
			Target: &model.CodeElement{
				Kind: targetKind,
				Name: targetName,
				QualifiedName: resolveQualifiedName(simpleID, filePath, gc), 
			},
			Location: nodeToLocation(simpleID, filePath),
		})
	}
	
	// 由于 Query 结构为 []， match 可能同时包含多个捕获，返回一个 relations 列表是正确的。
	return relations, nil
}

// --- 辅助函数 ---

// isPrimitiveType 判断是否为 Java 基本类型（为了简洁，这里只列出部分）
func isPrimitiveType(typeName string) bool {
	switch typeName {
	case "void", "int", "boolean", "char", "byte", "short", "long", "float", "double":
		return true
	default:
		return false
	}
}

// findCapturedNode 从匹配中查找特定名称的捕获节点
func findCapturedNode(match *sitter.QueryMatch, name string) *sitter.Node {
	for _, capture := range match.Captures {
		if match.Query.FieldNameForId(capture.Index) == name {
			return capture.Node
		}
	}
	return nil
}

// nodeToLocation 将 AST 节点转换为 Location 结构体
func nodeToLocation(n *sitter.Node, filePath string) *model.Location {
	return &model.Location{
		FilePath: filePath,
		StartLine: int(n.StartPoint().Row + 1),
		EndLine: int(n.EndPoint().Row + 1),
		StartColumn: int(n.StartPoint().Column + 1),
		EndColumn: int(n.EndPoint().Column + 1),
	}
}

// resolveQualifiedName 限定名解析
func resolveQualifiedName(n *sitter.Node, filePath string, gc *model.GlobalContext) string {
	name := n.Content()

	// 1. 尝试在当前文件的上下文中查找（本地查找）
	if fc, ok := gc.FileContexts[filePath]; ok {
		// 查找文件内定义 (如果 QN 已经是全限定名，这步可能失败)
		if entry, ok := fc.Definitions[name]; ok {
			return entry.Element.QualifiedName
		}
		
		// 尝试使用当前文件的 PackageName 作为前缀
		if fc.PackageName != "" {
			possibleQN := fmt.Sprintf("%s.%s", fc.PackageName, name)
			if gc.ResolveQN(possibleQN) != nil {
				return possibleQN
			}
		}
	}

	// 2. 尝试全局查找
	// 如果 name 能够直接匹配到 GlobalDefinitions 中的一个 QN，则返回
	// 这是一个非常宽松的检查，但可以解决一些跨包调用问题
	if definitions := gc.ResolveQN(name); len(definitions) > 0 {
		return name // 假设找到的第一个就是目标
	}

	// 3. 回退
	return fmt.Sprintf("UNKNOWN_QN:%s", name)
}

// getDefinitionElement 根据 AST 节点类型，返回对应的 CodeElement 定义和 Kind。
func getDefinitionElement(node *sitter.Node, filePath string) (*model.CodeElement, model.ElementKind) {
	switch node.Type() {
	case "class_declaration":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			return &model.CodeElement{
				Kind: model.Class,
				Name: nameNode.Content(),
				Path: filePath,
			}, model.Class
		}
	case "interface_declaration":
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			return &model.CodeElement{
				Kind: model.Interface,
				Name: nameNode.Content(),
				Path: filePath,
			}, model.Interface
		}
	case "method_declaration", "constructor_declaration":
		nameNode := node.ChildByFieldName("name")
		name := ""
		if nameNode != nil {
			name = nameNode.Content()
		}
		// 构造函数可能没有 'name' 字段，此时使用类名或特殊标记
		if name == "" && node.Type() == "constructor_declaration" {
			// 需要更复杂的逻辑来获取类名，这里简化为 Constructor
			name = "Constructor" 
		}

		if name != "" {
			return &model.CodeElement{
				Kind: model.Method,
				Name: name,
				Path: filePath,
			}, model.Method
		}
	case "field_declaration":
		// Tree-sitter 复杂字段声明的简化处理，只关注第一个 variable_declarator
		if vNode := node.NamedChild(1); vNode != nil && vNode.Type() == "variable_declarator" {
			if nameNode := vNode.ChildByFieldName("name"); nameNode != nil {
				return &model.CodeElement{
					Kind: model.Field,
					Name: nameNode.Content(),
					Path: filePath,
				}, model.Field
			}
		}
	case "package_declaration":
		// 包名已在 collectDefinitions 顶部处理
		if node.ChildCount() > 1 {
			return &model.CodeElement{
				Kind: model.Package,
				Name: node.Child(1).Content(),
				Path: filePath,
			}, model.Package
		}
	}
	
	return nil, ""
}

// determineSourceElement 向上遍历 AST，找到最近的逻辑块（函数/方法/初始化块），作为 CALL 关系的 Source。
func determineSourceElement(n *sitter.Node, filePath string, gc *model.GlobalContext) *model.CodeElement {
	// 从当前节点开始向上遍历到 AST 根节点
	for n != nil {
		switch n.Type() {
		
		// 1. 匹配方法声明 (method_declaration)
		case "method_declaration":
			// 提取方法名
			methodNameNode := n.ChildByFieldName("name")
			name := ""
			if methodNameNode != nil {
				name = methodNameNode.Content()
			}
			
			return &model.CodeElement{
				Kind: model.Method,
				Name: name,
				// 确定 QN：需要解析所属的 Class/Package
				QualifiedName: resolveQualifiedName(n, filePath, gc),
				Path: filePath,
			}
			
		// 2. 匹配构造函数声明 (constructor_declaration)
		case "constructor_declaration":
			// 构造函数名通常与类名相同
			return &model.CodeElement{
				Kind: model.Method, // 构造函数在依赖模型中视为一种特殊的方法
				Name: "Constructor",
				QualifiedName: resolveQualifiedName(n, filePath, gc),
				Path: filePath,
			}
			
		// 3. 匹配静态/实例初始化块 (block)
		case "block":
			// 检查块是否是静态初始化块或实例初始化块
			parentNode := n.Parent()
			if parentNode != nil && (parentNode.Type() == "static_initializer" || parentNode.Type() == "constructor_declaration") {
				// 对于静态或实例初始化块，将其 Source 定位到所属的 Class
				// 向上继续寻找 class_declaration
				continue 
			}
			
		// 4. 匹配 lambda 表达式 (lambda_expression)
		case "lambda_expression":
			return &model.CodeElement{
				Kind: model.Function, // Lambda 视为匿名函数
				Name: "lambda",
				QualifiedName: resolveQualifiedName(n, filePath, gc),
				Path: filePath,
			}
			
		// 5. 匹配类/接口声明 (class_declaration, interface_declaration)
		case "class_declaration", "interface_declaration":
			// 当调用发生在类/接口的顶层（如字段初始化表达式），我们将其 Source 定位到 Class/Interface
			nameNode := n.ChildByFieldName("name")
			kind := model.Class
			if n.Type() == "interface_declaration" {
				kind = model.Interface
			}
			
			return &model.CodeElement{
				Kind: kind,
				Name: nameNode.Content(),
				QualifiedName: resolveQualifiedName(n, filePath, gc),
				Path: filePath,
			}
			
		// 6. 匹配文件根节点 (program)
		case "program":
			// 达到文件顶部，Source 为文件本身
			return &model.CodeElement{
				Kind: model.File,
				Name: filePath,
				QualifiedName: filePath,
				Path: filePath,
			}
		}
		
		// 向上移动到父节点
		n = n.Parent()
	}
	
	// 理论上不会执行到这里，但作为安全回退
	return &model.CodeElement{
		Kind: model.File,
		Name: filePath,
		QualifiedName: filePath,
		Path: filePath,
	}
}