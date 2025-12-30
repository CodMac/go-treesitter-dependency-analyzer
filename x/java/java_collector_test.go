package java_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CodMac/go-treesitter-dependency-analyzer/parser"
	"github.com/CodMac/go-treesitter-dependency-analyzer/x/java"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	// 导入所有语言绑定，确保 GetLanguage 可以找到
	_ "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

func getTestFilePath(name string) string {
	currentDir, _ := filepath.Abs(filepath.Dir("."))
	return filepath.Join(currentDir, "testdata", name)
}

// 将返回值类型改为接口 parser.Parser
func getJavaParser(t *testing.T) parser.Parser {
	javaParser, err := parser.NewParser(model.LangJava)
	if err != nil {
		t.Fatalf("Failed to create Java parser: %v", err)
	}

	return javaParser
}

const printEle = true

func printCodeElements(fCtx *model.FileContext) {
	if !printEle {
		return
	}

	fmt.Printf("Package: %s\n", fCtx.PackageName)
	for _, defs := range fCtx.DefinitionsBySN {
		for _, def := range defs {
			fmt.Printf("Short: %s  ->  Kind: %s,  QN: %s\n", def.Element.Name, def.Element.Kind, def.Element.QualifiedName)
		}
	}
}

// 验证注解定义、元注解提取
// 验证语义化 Import
// 验证注释提取
func TestJavaCollector_LoggableAnnotation(t *testing.T) {
	// 1. 获取测试文件路径 (对应 x/java/testdata/com/example/annotation/Loggable.java)
	filePath := getTestFilePath(filepath.Join("com", "example", "annotation", "Loggable.java"))

	// 2. 解析源码
	rootNode, sourceBytes, err := getJavaParser(t).ParseFile(filePath, true, true)
	if err != nil {
		t.Fatalf("Failed to parse file: %v", err)
	}

	// 3. 运行 Collector
	collector := java.NewJavaCollector()
	fCtx, err := collector.CollectDefinitions(rootNode, filePath, sourceBytes)
	if err != nil {
		t.Fatalf("CollectDefinitions failed: %v", err)
	}
	printCodeElements(fCtx)

	// 4. 验证 Package Name
	expectedPackage := "com.example.annotation"
	if fCtx.PackageName != expectedPackage {
		t.Errorf("Expected PackageName %q, got %q", expectedPackage, fCtx.PackageName)
	}

	// 5. 验证语义化 Import (修正重点：从 map[string]string 变为 ImportEntry)
	t.Run("Verify Semantic Imports", func(t *testing.T) {
		imp, ok := fCtx.Imports["*"]
		if !ok {
			t.Fatal("Expected wildcard import '*' not found")
		}
		if imp.RawImportPath != "java.lang.annotation.*" {
			t.Errorf("Import path mismatch: expected java.lang.annotation.*, got %s", imp.RawImportPath)
		}
		if !imp.IsWildcard {
			t.Error("Expected IsWildcard to be true")
		}
		if imp.Kind != model.Package {
			t.Errorf("Expected Kind %s, got %s", model.Package, imp.Kind)
		}
	})

	// 6. 验证注解定义与注释 (修正重点：增加 Doc 验证)
	t.Run("Verify Annotation and Doc", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN["Loggable"]
		if len(defs) == 0 {
			t.Fatal("Annotation 'Loggable' not found")
		}

		elem := defs[0].Element
		if elem.Kind != model.KAnnotation {
			t.Errorf("Expected Kind %s, got %s", model.KAnnotation, elem.Kind)
		}

		// 验证 Doc 注释提取
		if !strings.Contains(elem.Doc, "测试：Annotation Type Declaration") {
			t.Errorf("Doc mismatch, got: %q", elem.Doc)
		}

		// 验证元注解提取
		if elem.Extra == nil || len(elem.Extra.Annotations) == 0 {
			t.Error("Extra.Annotations should not be empty")
		}
	})

	// 7. 验证注解属性 (修正重点：现在应该能通过了)
	t.Run("Verify Annotation Properties", func(t *testing.T) {
		properties := []struct {
			name string
			ret  string
			sign string
		}{
			{"level", "String", "String level()"},
			{"trace", "boolean", "boolean trace()"},
		}

		for _, prop := range properties {
			defs := fCtx.DefinitionsBySN[prop.name]
			if len(defs) == 0 {
				t.Errorf("Annotation property %q not found. Check if node type 'annotation_type_element_declaration' is handled.", prop.name)
				continue
			}

			elem := defs[0].Element
			if elem.Kind != model.Method {
				t.Errorf("Property %q: expected Kind METHOD, got %s", prop.name, elem.Kind)
			}

			// 验证返回类型 (存在于 Extra 和 Signature 中)
			if elem.Extra == nil || elem.Extra.MethodExtra.ReturnType != prop.ret {
				t.Errorf("Property %q: expected return type %q, got %v", prop.name, prop.ret, elem.Extra.MethodExtra.ReturnType)
			}

			// 验证细化后的 Signature
			if !strings.Contains(elem.Signature, prop.sign) {
				t.Errorf("Property %q: signature mismatch. Expected contains %q, got %q", prop.name, prop.sign, elem.Signature)
			}
		}
	})

	fmt.Println("Java Collector Loggable test completed successfully.")
}

// 验证包名与导入
// 验证顶级类定义
// 验证字段与方法
// 验证嵌套内部类 (最关键的 QN 逻辑)
func TestJavaCollector_AbstractBaseEntity(t *testing.T) {
	// 1. 获取测试文件路径
	filePath := getTestFilePath(filepath.Join("com", "example", "model", "AbstractBaseEntity.java"))

	// 2. 解析源码
	rootNode, sourceBytes, err := getJavaParser(t).ParseFile(filePath, true, true)
	if err != nil {
		t.Fatalf("Failed to parse file: %v", err)
	}

	// 3. 运行 Collector
	collector := java.NewJavaCollector()
	fCtx, err := collector.CollectDefinitions(rootNode, filePath, sourceBytes)
	if err != nil {
		t.Fatalf("CollectDefinitions failed: %v", err)
	}
	printCodeElements(fCtx)

	// 4. 验证包名与导入
	assert.Equal(t, "com.example.model", fCtx.PackageName)
	assert.Contains(t, fCtx.Imports, "Serializable")
	assert.Contains(t, fCtx.Imports, "Date")

	// 5. 验证顶级类定义
	t.Run("Verify Top Level Class", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN["AbstractBaseEntity"]
		require.NotEmpty(t, defs)

		elem := defs[0].Element
		assert.Equal(t, model.Class, elem.Kind)
		assert.Equal(t, "com.example.model.AbstractBaseEntity", elem.QualifiedName)

		// 验证 ClassExtra (继承与实现)
		require.NotNil(t, elem.Extra.ClassExtra)
		// 注意：Java AST 可能会保留泛型符号 <ID>，根据你目前的 getNodeContent 逻辑判断
		assert.Contains(t, elem.Extra.ClassExtra.ImplementedInterfaces, "Serializable")
		assert.Contains(t, elem.Extra.Modifiers, "public")
		assert.Contains(t, elem.Extra.Modifiers, "abstract")
	})

	// 6. 验证字段与方法
	t.Run("Verify Members", func(t *testing.T) {
		// 验证字段 id
		idDefs := fCtx.DefinitionsBySN["id"]
		require.NotEmpty(t, idDefs)
		assert.Equal(t, "com.example.model.AbstractBaseEntity.id", idDefs[0].Element.QualifiedName)
		assert.Equal(t, "ID", idDefs[0].Element.Extra.FieldExtra.Type)

		// 验证方法 getId
		getDefs := fCtx.DefinitionsBySN["getId"]
		require.NotEmpty(t, getDefs)
		assert.Equal(t, "com.example.model.AbstractBaseEntity.getId()", getDefs[0].Element.QualifiedName)
		assert.Equal(t, "ID", getDefs[0].Element.Extra.MethodExtra.ReturnType)
	})

	// 7. 验证嵌套内部类 (最关键的 QN 逻辑)
	t.Run("Verify Nested Inner Class", func(t *testing.T) {
		// 验证内部类 EntityMeta
		metaDefs := fCtx.DefinitionsBySN["EntityMeta"]
		require.NotEmpty(t, metaDefs)

		metaElem := metaDefs[0].Element
		assert.Equal(t, model.Class, metaElem.Kind)
		// 验证 QN 是否正确拼接了父类名
		expectedMetaQN := "com.example.model.AbstractBaseEntity.EntityMeta"
		assert.Equal(t, expectedMetaQN, metaElem.QualifiedName)

		// 验证内部类的成员
		tableDefs := fCtx.DefinitionsBySN["tableName"]
		require.NotEmpty(t, tableDefs)
		assert.Equal(t, expectedMetaQN+".tableName", tableDefs[0].Element.QualifiedName)
		assert.Equal(t, "String", tableDefs[0].Element.Extra.FieldExtra.Type)
	})

	// 8. 验证 Doc 采集
	t.Run("Verify Class Doc", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN["AbstractBaseEntity"]
		if len(defs) > 0 {
			assert.Contains(t, defs[0].Element.Doc, "基础实体类")
		}
	})
}

// 验证接口继承
// 验证函数全路径名称
// 验证方法异常抛出
// 验证默认方法与修饰符
func TestJavaCollector_DataProcessor_Complex(t *testing.T) {
	// 1. 初始化 (保持不变)
	filePath := getTestFilePath(filepath.Join("com", "example", "core", "DataProcessor.java"))
	rootNode, sourceBytes, err := getJavaParser(t).ParseFile(filePath, true, false)
	require.NoError(t, err)

	collector := java.NewJavaCollector()
	fCtx, err := collector.CollectDefinitions(rootNode, filePath, sourceBytes)
	require.NoError(t, err)
	printCodeElements(fCtx)

	const interfaceQN = "com.example.core.DataProcessor"

	// 2. 验证接口继承 (保持不变并补充 QN 校验)
	t.Run("Verify Multiple Interface Inheritance", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN["DataProcessor"]
		require.NotEmpty(t, defs)

		elem := defs[0].Element
		assert.Equal(t, interfaceQN, elem.QualifiedName) // 校验接口 QN

		ce := elem.Extra.ClassExtra
		require.NotNil(t, ce)
		assert.Contains(t, ce.ImplementedInterfaces, "Runnable")
		assert.Contains(t, ce.ImplementedInterfaces, "AutoCloseable")
		assert.Contains(t, elem.Doc, "Method Throws")
	})

	// 3. 验证函数全路径名称 (核心补充：processAll)
	t.Run("Verify Method Full Qualified Names", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN["processAll"]
		require.NotEmpty(t, defs)

		elem := defs[0].Element
		me := elem.Extra.MethodExtra
		require.NotNil(t, me)

		// 验证 QualifiedName: 包含类型，不包含参数名，擦除泛型 (如果有)
		// 期望: 包名.类名.方法名(参数类型)
		expectedQN := interfaceQN + ".processAll(String)"
		assert.Equal(t, expectedQN, elem.QualifiedName)

		// 验证 IncludeParamNameQN: 包含参数类型和参数名
		// 期望: 包名.类名.方法名(参数类型 参数名)
		expectedFullQN := interfaceQN + ".processAll(String batchId)"
		assert.Equal(t, expectedFullQN, me.IncludeParamNameQN)

		// 额外验证返回类型
		assert.Equal(t, "List<T>", me.ReturnType)
	})

	// 4. 验证默认方法全路径 (stop)
	t.Run("Verify Default Method QN", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN["stop"]
		require.NotEmpty(t, defs)

		elem := defs[0].Element
		me := elem.Extra.MethodExtra
		require.NotNil(t, me)

		// 无参方法 QN
		assert.Equal(t, interfaceQN+".stop()", elem.QualifiedName)
		// 无参方法 IncludeParamNameQN 与 QN 一致
		assert.Equal(t, interfaceQN+".stop()", me.IncludeParamNameQN)

		assert.Contains(t, elem.Extra.Modifiers, "default")
	})

	// 5. 验证导入表 (保持不变)
	t.Run("Verify Imports", func(t *testing.T) {
		assert.Contains(t, fCtx.Imports, "AbstractBaseEntity")
		assert.Equal(t, "com.example.model.AbstractBaseEntity", fCtx.Imports["AbstractBaseEntity"].RawImportPath)
	})
}

// 验证类级别的元数据 (泛型继承与注解)
// 验证构造函数 (Constructor)
// 验证复杂方法签名与异常
// 验证字段采集
func TestJavaCollector_UserServiceImpl(t *testing.T) {
	// 1. 获取测试文件路径
	filePath := getTestFilePath(filepath.Join("com", "example", "service", "UserServiceImpl.java"))

	// 2. 解析源码
	rootNode, sourceBytes, err := getJavaParser(t).ParseFile(filePath, true, true)
	require.NoError(t, err)

	// 3. 运行 Collector
	collector := java.NewJavaCollector()
	fCtx, err := collector.CollectDefinitions(rootNode, filePath, sourceBytes)
	require.NoError(t, err)
	printCodeElements(fCtx)

	// 4. 验证类级别的元数据 (泛型继承与注解)
	t.Run("Verify Class Metadata", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN["UserServiceImpl"]
		require.NotEmpty(t, defs)
		elem := defs[0].Element

		assert.Equal(t, "com.example.service.UserServiceImpl", elem.QualifiedName)

		// 验证注解采集
		assert.Contains(t, elem.Extra.Annotations, "@Loggable")

		// 验证继承 (含泛型)
		ce := elem.Extra.ClassExtra
		require.NotNil(t, ce)
		assert.Equal(t, "AbstractBaseEntity<String>", ce.SuperClass)

		// 验证接口实现 (DataProcessor 同样带泛型)
		assert.Contains(t, ce.ImplementedInterfaces, "DataProcessor<AbstractBaseEntity<String>>")
	})

	// 5. 验证构造函数 (Constructor)
	t.Run("Verify Constructor", func(t *testing.T) {
		// SN 应该匹配类名 UserServiceImpl
		defs := fCtx.DefinitionsBySN["UserServiceImpl"]
		var constructor *model.CodeElement
		for _, d := range defs {
			if d.Element.Extra.MethodExtra != nil && d.Element.Extra.MethodExtra.IsConstructor {
				constructor = d.Element
				break
			}
		}
		require.NotNil(t, constructor, "Constructor not found")
		assert.True(t, constructor.Extra.MethodExtra.IsConstructor)
		assert.Equal(t, "public UserServiceImpl()", constructor.Signature)
	})

	// 6. 验证复杂方法签名与异常
	t.Run("Verify Complex Method", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN["processAll"]
		require.NotEmpty(t, defs)
		elem := defs[0].Element

		assert.Equal(t, "List<AbstractBaseEntity<String>>", elem.Extra.MethodExtra.ReturnType)
		assert.Contains(t, elem.Extra.MethodExtra.ThrowsTypes, "RuntimeException")

		// 验证 @Override 是否被正确采集为修饰符或注解
		assert.Condition(t, func() bool {
			return contains(elem.Extra.Annotations, "@Override")
		}, "Should contain @Override")
	})

	// 7. 验证字段采集
	t.Run("Verify Private Field", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN["cache"]
		require.NotEmpty(t, defs)
		fe := defs[0].Element.Extra.FieldExtra

		assert.Equal(t, "List<String>", fe.Type)
		assert.Contains(t, defs[0].Element.Extra.Modifiers, "final")
	})
}

// 验证 Enum 定义
// 验证 Enum Constants (枚举常量)
// 验证内部字段 (Fields)
// 验证构造函数 (Enum Constructor)
func TestJavaCollector_ErrorCode(t *testing.T) {
	// 1. 获取测试文件路径
	filePath := getTestFilePath(filepath.Join("com", "example", "model", "ErrorCode.java"))

	// 2. 解析源码
	rootNode, sourceBytes, err := getJavaParser(t).ParseFile(filePath, true, false)
	require.NoError(t, err)

	// 3. 运行 Collector
	collector := java.NewJavaCollector()
	fCtx, err := collector.CollectDefinitions(rootNode, filePath, sourceBytes)
	require.NoError(t, err)
	printCodeElements(fCtx)

	// 4. 验证 Enum 定义
	t.Run("Verify Enum Definition", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN["ErrorCode"]
		require.NotEmpty(t, defs)

		elem := defs[0].Element
		assert.Equal(t, model.Enum, elem.Kind)
		assert.Equal(t, "com.example.model.ErrorCode", elem.QualifiedName)
		assert.Contains(t, elem.Doc, "用于测试 ENUM 定义和使用")
	})

	// 5. 验证 Enum Constants (枚举常量)
	t.Run("Verify Enum Constants", func(t *testing.T) {
		constants := []string{"USER_NOT_FOUND", "NAME_EMPTY"}
		for _, name := range constants {
			defs := fCtx.DefinitionsBySN[name]
			require.NotEmpty(t, defs, "Enum constant %s not found", name)

			elem := defs[0].Element
			assert.Equal(t, model.EnumConstant, elem.Kind)
			assert.Equal(t, "com.example.model.ErrorCode."+name, elem.QualifiedName)
		}
	})

	// 6. 验证内部字段 (Fields)
	t.Run("Verify Internal Fields", func(t *testing.T) {
		fields := []struct {
			name string
			typ  string
		}{
			{"code", "int"},
			{"message", "String"},
		}

		for _, f := range fields {
			defs := fCtx.DefinitionsBySN[f.name]
			require.NotEmpty(t, defs)

			elem := defs[0].Element
			assert.Equal(t, model.Field, elem.Kind)
			assert.Equal(t, f.typ, elem.Extra.FieldExtra.Type)
			assert.Contains(t, elem.Extra.Modifiers, "private")
			assert.Contains(t, elem.Extra.Modifiers, "final")
		}
	})

	// 7. 验证构造函数 (Enum Constructor)
	t.Run("Verify Enum Constructor", func(t *testing.T) {
		// 枚举构造函数的 SN 应该是枚举类名 ErrorCode
		defs := fCtx.DefinitionsBySN["ErrorCode"]
		var constructor *model.CodeElement
		for _, d := range defs {
			if d.Element.Kind == model.Method && d.Element.Extra.MethodExtra != nil && d.Element.Extra.MethodExtra.IsConstructor {
				constructor = d.Element
				break
			}
		}
		require.NotNil(t, constructor, "Enum constructor not found")
		// 注意：Java 枚举构造函数默认是 private 的，即便不写
		assert.True(t, constructor.Extra.MethodExtra.IsConstructor)
		assert.Equal(t, 2, len(constructor.Extra.MethodExtra.Parameters))
	})

	// 8. 验证普通方法 (Methods)
	t.Run("Verify Methods", func(t *testing.T) {
		methods := []string{"getCode", "getMessage"}
		for _, name := range methods {
			defs := fCtx.DefinitionsBySN[name]
			require.NotEmpty(t, defs)

			elem := defs[0].Element
			assert.Equal(t, model.Method, elem.Kind)
			assert.Contains(t, elem.Extra.Modifiers, "public")
			assert.Equal(t, "com.example.model.ErrorCode."+name+"()", elem.QualifiedName)
		}
	})

	// 9. 验证枚举常量的内部参数提取 (New!)
	t.Run("Verify Enum Constant Arguments", func(t *testing.T) {
		testCases := []struct {
			name         string
			expectedArgs []string
		}{
			{"USER_NOT_FOUND", []string{"404", "\"User not found in repository\""}},
			{"NAME_EMPTY", []string{"400", "\"User name cannot be empty\""}},
		}

		for _, tc := range testCases {
			defs := fCtx.DefinitionsBySN[tc.name]
			require.NotEmpty(t, defs)

			elem := defs[0].Element
			require.NotNil(t, elem.Extra.EnumConstantExtra, "EnumConstantExtra should not be nil for %s", tc.name)

			// 验证参数内容
			assert.Equal(t, tc.expectedArgs, elem.Extra.EnumConstantExtra.Arguments)
		}
	})
}

// 验证类定义与继承关系
// 验证字段
// 验证构造函数重载
func TestJavaCollector_NotificationException(t *testing.T) {
	// 1. 获取测试文件路径
	filePath := getTestFilePath(filepath.Join("com", "example", "model", "NotificationException.java"))

	// 2. 解析源码
	rootNode, sourceBytes, err := getJavaParser(t).ParseFile(filePath, true, true)
	if err != nil {
		t.Fatalf("Failed to parse file: %v", err)
	}

	// 3. 运行 Collector
	collector := java.NewJavaCollector()
	fCtx, err := collector.CollectDefinitions(rootNode, filePath, sourceBytes)
	if err != nil {
		t.Fatalf("CollectDefinitions failed: %v", err)
	}
	printCodeElements(fCtx)

	// 4. 验证 Package Name
	expectedPackage := "com.example.model"
	if fCtx.PackageName != expectedPackage {
		t.Errorf("Expected PackageName %q, got %q", expectedPackage, fCtx.PackageName)
	}

	// 5. 验证类定义与继承关系 (EXTEND 关系)
	className := "NotificationException"
	classQN := "com.example.model.NotificationException"
	t.Run("Verify Class and Inheritance", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN[className]
		if len(defs) == 0 {
			t.Fatalf("Class %q not found", className)
		}

		elem := defs[0].Element
		if elem.Kind != model.Class {
			t.Errorf("Expected Kind CLASS, got %s", elem.Kind)
		}
		assert.Equal(t, classQN, elem.QualifiedName)

		// 验证父类提取 (extends Exception)
		if elem.Extra == nil || elem.Extra.ClassExtra == nil {
			t.Fatal("Extra.ClassExtra is nil")
		}
		if elem.Extra.ClassExtra.SuperClass != "Exception" {
			t.Errorf("Expected SuperClass 'Exception', got %q", elem.Extra.ClassExtra.SuperClass)
		}

		// 验证 Doc
		if !strings.Contains(elem.Doc, "用于测试 EXTEND 和 THROW 关系") {
			t.Errorf("Doc mismatch, got: %q", elem.Doc)
		}
	})

	// 6. 验证字段 (CONTAIN 关系)
	t.Run("Verify Fields", func(t *testing.T) {
		fieldName := "serialVersionUID"
		defs := fCtx.DefinitionsBySN[fieldName]
		if len(defs) == 0 {
			t.Fatalf("Field %q not found", fieldName)
		}

		elem := defs[0].Element
		// 验证修饰符: private static final
		modifiers := elem.Extra.Modifiers
		expectedMods := []string{"private", "static", "final"}
		for _, mod := range expectedMods {
			found := false
			for _, m := range modifiers {
				if m == mod {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Modifier %q not found in %v", mod, modifiers)
			}
		}

		// 验证字段类型
		if elem.Extra.FieldExtra.Type != "long" {
			t.Errorf("Expected field type 'long', got %q", elem.Extra.FieldExtra.Type)
		}
	})

	// 7. 验证构造函数重载 (USE 关系)
	t.Run("Verify Overloaded Constructors QN", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN[className]
		var constructors []*model.CodeElement
		for _, d := range defs {
			if d.Element.Kind == model.Method {
				constructors = append(constructors, d.Element)
			}
		}

		if len(constructors) != 2 {
			t.Errorf("Expected 2 constructors, found %d", len(constructors))
		}

		foundTwoParamCtor := false
		foundOneParamCtor := false

		for _, c := range constructors {
			me := c.Extra.MethodExtra
			require.NotNil(t, me)

			// 场景 A: NotificationException(String message, Throwable cause)
			if len(me.Parameters) == 2 {
				foundTwoParamCtor = true

				// 校验 QualifiedName: 擦除泛型，仅保留类型
				// 注意：java.lang.String 在源码中写为 String，按 collector 逻辑会提取为 String
				expectedQN := classQN + ".NotificationException(String,Throwable)"
				assert.Equal(t, expectedQN, c.QualifiedName, "Two-param constructor QN mismatch")

				// 校验 IncludeParamNameQN: 包含参数名
				expectedFullQN := classQN + ".NotificationException(String message, Throwable cause)"
				assert.Equal(t, expectedFullQN, me.IncludeParamNameQN, "Two-param constructor Full QN mismatch")
			}

			// 场景 B: NotificationException(ErrorCode code)
			if len(me.Parameters) == 1 {
				foundOneParamCtor = true

				// 校验 QualifiedName
				expectedQN := classQN + ".NotificationException(ErrorCode)"
				assert.Equal(t, expectedQN, c.QualifiedName, "One-param constructor QN mismatch")

				// 校验 IncludeParamNameQN
				expectedFullQN := classQN + ".NotificationException(ErrorCode code)"
				assert.Equal(t, expectedFullQN, me.IncludeParamNameQN, "One-param constructor Full QN mismatch")
			}
		}

		assert.True(t, foundTwoParamCtor, "Constructor (String, Throwable) not found")
		assert.True(t, foundOneParamCtor, "Constructor (ErrorCode) not found")
	})

	fmt.Println("Java Collector NotificationException test completed successfully.")
}

// 验证静态导入
// 验证类 User 成员
// 验证内部类
// 验证内部类的私有静态方法
func TestJavaCollector_User(t *testing.T) {
	// 1. 获取测试文件路径
	filePath := getTestFilePath(filepath.Join("com", "example", "model", "User.java"))

	// 2. 解析源码
	rootNode, sourceBytes, err := getJavaParser(t).ParseFile(filePath, true, true)
	if err != nil {
		t.Fatalf("Failed to parse file: %v", err)
	}

	// 3. 运行 Collector
	collector := java.NewJavaCollector()
	fCtx, err := collector.CollectDefinitions(rootNode, filePath, sourceBytes)
	if err != nil {
		t.Fatalf("CollectDefinitions failed: %v", err)
	}
	printCodeElements(fCtx)

	// 4. 验证 Package 和 Imports
	t.Run("Verify Package and Static Imports", func(t *testing.T) {
		expectedPackage := "com.example.model"
		if fCtx.PackageName != expectedPackage {
			t.Errorf("Expected PackageName %q, got %q", expectedPackage, fCtx.PackageName)
		}

		// 验证静态导入 (例如 DAYS)
		if imp, ok := fCtx.Imports["DAYS"]; ok {
			if imp.Kind != model.Constant {
				t.Errorf("Expected DAYS to be Kind CONSTANT, got %s", imp.Kind)
			}
			if imp.RawImportPath != "java.util.concurrent.TimeUnit.DAYS" {
				t.Errorf("Wrong import path for DAYS: %s", imp.RawImportPath)
			}
		} else {
			t.Error("Static import 'DAYS' not found")
		}
	})

	// 5. 验证外部类 User 成员
	const classQN = "com.example.model.User"
	const addonInfoQN = "com.example.model.User.AddonInfo"
	// 5. 验证外部类 User 成员 (补充方法 QN 校验)
	t.Run("Verify User Method and Constructor QNs", func(t *testing.T) {
		// A. 验证构造函数 User(String username)
		if defs := fCtx.DefinitionsBySN["User"]; len(defs) > 0 {
			var ctor *model.CodeElement
			for _, d := range defs {
				if d.Element.Kind == model.Method { // 构造函数在 collector 中标记为 Method
					ctor = d.Element
					break
				}
			}
			require.NotNil(t, ctor)
			// QN: 稳定索引
			assert.Equal(t, classQN+".User(String)", ctor.QualifiedName)
			// IncludeParamNameQN: 带参数名
			assert.Equal(t, classQN+".User(String username)", ctor.Extra.MethodExtra.IncludeParamNameQN)
		}

		// B. 验证 setUsername(String username)
		if defs := fCtx.DefinitionsBySN["setUsername"]; len(defs) > 0 {
			elem := defs[0].Element
			assert.Equal(t, classQN+".setUsername(String)", elem.QualifiedName)
			assert.Equal(t, classQN+".setUsername(String username)", elem.Extra.MethodExtra.IncludeParamNameQN)
		}

		// C. 验证 getId() (无参)
		if defs := fCtx.DefinitionsBySN["getId"]; len(defs) > 0 {
			elem := defs[0].Element
			assert.Equal(t, classQN+".getId()", elem.QualifiedName)
			assert.Equal(t, classQN+".getId()", elem.Extra.MethodExtra.IncludeParamNameQN)
		}
	})

	// 6. 验证内部类 AddonInfo (Nested Class)
	t.Run("Verify Nested Class AddonInfo", func(t *testing.T) {
		innerClassName := "AddonInfo"
		defs := fCtx.DefinitionsBySN[innerClassName]
		if len(defs) == 0 {
			t.Fatalf("Nested class %q not found", innerClassName)
		}

		elem := defs[0].Element
		expectedQN := "com.example.model.User.AddonInfo"
		if elem.QualifiedName != expectedQN {
			t.Errorf("Expected QN %q, got %q", expectedQN, elem.QualifiedName)
		}

		// 验证内部类的字段及其多种修饰符
		fieldTests := []struct {
			name string
			mod  string // 预期包含的修饰符，空字符串代表 default
		}{
			{"otherName", "public"},
			{"age", "private"},
			{"birthday", "protected"},
			{"workTimeUnit", ""}, // Default access
		}

		for _, ft := range fieldTests {
			fDefs := fCtx.DefinitionsBySN[ft.name]
			// 过滤出属于 AddonInfo 的字段
			var target *model.CodeElement
			for _, fd := range fDefs {
				if strings.HasPrefix(fd.Element.QualifiedName, expectedQN) {
					target = fd.Element
					break
				}
			}

			if target == nil {
				t.Errorf("Field %q in AddonInfo not found", ft.name)
				continue
			}

			if ft.mod != "" && !contains(target.Extra.Modifiers, ft.mod) {
				t.Errorf("Field %q expected modifier %q, got %v", ft.name, ft.mod, target.Extra.Modifiers)
			}
		}
	})

	// 7. 验证内部类的私有静态方法
	t.Run("Verify Private Static Method QN in Nested Class", func(t *testing.T) {
		methodName := "chooseUnit"
		defs := fCtx.DefinitionsBySN[methodName]
		require.NotEmpty(t, defs)

		elem := defs[0].Element
		// QN 路径应包含完整的内部类路径
		assert.Equal(t, addonInfoQN+".chooseUnit(long)", elem.QualifiedName)
		assert.Equal(t, addonInfoQN+".chooseUnit(long nanos)", elem.Extra.MethodExtra.IncludeParamNameQN)

		if elem.Extra.MethodExtra.ReturnType != "TimeUnit" {
			t.Errorf("Expected return type TimeUnit, got %s", elem.Extra.MethodExtra.ReturnType)
		}
	})

	fmt.Println("Java Collector User (with Nested Class) test completed successfully.")
}

// 验证变长参数与数组类型
//
//	验证复杂注解提取
func TestJavaCollector_ConfigService(t *testing.T) {
	// 1. 获取测试文件路径
	filePath := getTestFilePath(filepath.Join("com", "example", "config", "ConfigService.java"))

	// 2. 解析源码
	rootNode, sourceBytes, err := getJavaParser(t).ParseFile(filePath, true, true)
	if err != nil {
		t.Fatalf("Failed to parse file: %v", err)
	}

	// 3. 运行 Collector
	collector := java.NewJavaCollector()
	fCtx, err := collector.CollectDefinitions(rootNode, filePath, sourceBytes)
	if err != nil {
		t.Fatalf("CollectDefinitions failed: %v", err)
	}
	printCodeElements(fCtx)

	// 4. 验证 Package
	expectedPackage := "com.example.config"
	if fCtx.PackageName != expectedPackage {
		t.Errorf("Expected PackageName %q, got %q", expectedPackage, fCtx.PackageName)
	}

	// 5. 验证变长参数与数组类型 (补充 QN 校验)
	const classQN = "com.example.config.ConfigService"
	t.Run("Verify Arrays and Varargs QN", func(t *testing.T) {
		methodName := "updateConfigs"
		defs := fCtx.DefinitionsBySN[methodName]
		require.NotEmpty(t, defs)

		elem := defs[0].Element
		mExtra := elem.Extra.MethodExtra
		require.NotNil(t, mExtra)

		// 验证 QualifiedName (用于稳定索引)
		// 数组应保留 [], 变长参数应保留 ...
		// 期望: 包名.类名.方法名(String[],Object...)
		expectedQN := classQN + ".updateConfigs(String[],Object...)"
		assert.Equal(t, expectedQN, elem.QualifiedName, "QN should correctly represent arrays and varargs")

		// 验证 IncludeParamNameQN (用于详细展示)
		// 期望: 包名.类名.方法名(String[] keys, Object... values)
		expectedFullQN := classQN + ".updateConfigs(String[] keys, Object... values)"
		assert.Equal(t, expectedFullQN, mExtra.IncludeParamNameQN, "Full QN should include parameter names with types")
	})

	// 6. 验证复杂注解方法全路径 (补充 legacyMethod 校验)
	t.Run("Verify Annotated Method QN", func(t *testing.T) {
		methodName := "legacyMethod"
		defs := fCtx.DefinitionsBySN[methodName]
		require.NotEmpty(t, defs)

		elem := defs[0].Element
		mExtra := elem.Extra.MethodExtra

		// 无参方法 QN
		assert.Equal(t, classQN+".legacyMethod()", elem.QualifiedName)
		assert.Equal(t, classQN+".legacyMethod()", mExtra.IncludeParamNameQN)
	})

	fmt.Println("Java Collector ConfigService boundary test completed successfully.")
}

// 验证接口定义与多重泛型边界
// 验证复杂泛型参数的方法 (List<? extends T>)
// 验证方法级别的泛型与抛出异常 (Method-level Generics)
func TestJavaCollector_GenericRepository(t *testing.T) {
	// 1. 获取测试文件路径
	filePath := getTestFilePath(filepath.Join("com", "example", "repo", "GenericRepository.java"))

	// 2. 解析源码
	rootNode, sourceBytes, err := getJavaParser(t).ParseFile(filePath, true, true)
	if err != nil {
		t.Fatalf("Failed to parse file: %v", err)
	}

	// 3. 运行 Collector
	collector := java.NewJavaCollector()
	fCtx, err := collector.CollectDefinitions(rootNode, filePath, sourceBytes)
	if err != nil {
		t.Fatalf("CollectDefinitions failed: %v", err)
	}
	printCodeElements(fCtx)

	// 4. 验证接口定义与多重泛型边界
	const interfaceQN = "com.example.repo.GenericRepository"
	t.Run("Verify Interface with Multiple Bounds", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN["GenericRepository"]
		if len(defs) == 0 {
			t.Fatal("Interface 'GenericRepository' not found")
		}

		elem := defs[0].Element
		if elem.Kind != model.Interface {
			t.Errorf("Expected Kind INTERFACE, got %s", elem.Kind)
		}
		assert.Equal(t, interfaceQN, elem.QualifiedName)

		// 验证 Doc
		if !strings.Contains(elem.Doc, "Serializable 和 Cloneable") {
			t.Errorf("Doc mismatch: %q", elem.Doc)
		}

		// 【补全验证】：验证泛型定义是否被包含在 Signature 中
		// 理想情况下，Signature 应为 "public interface GenericRepository<T extends Serializable & Cloneable>"
		expectedPart := "<T extends Serializable & Cloneable>"
		if !strings.Contains(elem.Signature, expectedPart) {
			t.Errorf("Interface signature missing generic parameters.\nExpected part: %q\nActual: %q",
				expectedPart, elem.Signature)
		}

		// 如果你将泛型信息存储在 ClassExtra 中，也可以增加如下断言
		// assert.Contains(t, elem.Extra.ClassExtra.TypeParameters, "T extends Serializable & Cloneable")
	})

	// 5. 验证复杂泛型参数的方法 (补充 QN 校验)
	t.Run("Verify Wildcard Generics QN", func(t *testing.T) {
		methodName := "findAllByCriteria"
		defs := fCtx.DefinitionsBySN[methodName]
		require.NotEmpty(t, defs)

		elem := defs[0].Element
		mExtra := elem.Extra.MethodExtra
		require.NotNil(t, mExtra)

		// 验证 QualifiedName: 擦除通配符和泛型，保留基础类型
		// 期望: ...GenericRepository.findAllByCriteria(List)
		expectedQN := interfaceQN + ".findAllByCriteria(List)"
		assert.Equal(t, expectedQN, elem.QualifiedName, "QN should erase wildcards")

		// 验证 IncludeParamNameQN: 保留完整泛型通配符和参数名
		// 期望: ...GenericRepository.findAllByCriteria(List<? super T> criteria)
		expectedFullQN := interfaceQN + ".findAllByCriteria(List<? super T> criteria)"
		assert.Equal(t, expectedFullQN, mExtra.IncludeParamNameQN)
	})

	// 6. 验证方法级别的泛型与抛出异常 (补充 QN 校验)
	t.Run("Verify Method Generics and Throws QN", func(t *testing.T) {
		methodName := "executeOrThrow"
		defs := fCtx.DefinitionsBySN[methodName]
		require.NotEmpty(t, defs)

		elem := defs[0].Element
		mExtra := elem.Extra.MethodExtra
		require.NotNil(t, mExtra)

		// 验证 QualifiedName: 方法级别的泛型 E 会被擦除为其上界（此处为 Object 或 E）
		// 根据 collector 逻辑 strings.Split(tStr, "<")[0]，泛型 E 会保留 E
		expectedQN := interfaceQN + ".executeOrThrow(E)"
		assert.Equal(t, expectedQN, elem.QualifiedName)

		// 验证 IncludeParamNameQN: 保留参数名
		expectedFullQN := interfaceQN + ".executeOrThrow(E exception)"
		assert.Equal(t, expectedFullQN, mExtra.IncludeParamNameQN)
	})

	fmt.Println("Java Collector GenericRepository test completed successfully.")
}

// 验证 Record 特性
// 验证 Sealed Interface
// 验证实现类
func TestJavaCollector_ModernJava(t *testing.T) {
	// 1. 获取测试文件路径
	filePath := getTestFilePath(filepath.Join("com", "example", "modern", "DataModel.java"))

	// 2. 解析源码
	rootNode, sourceBytes, err := getJavaParser(t).ParseFile(filePath, true, false)
	if err != nil {
		t.Fatalf("Failed to parse file: %v", err)
	}

	// 3. 运行 Collector
	collector := java.NewJavaCollector()
	fCtx, err := collector.CollectDefinitions(rootNode, filePath, sourceBytes)
	if err != nil {
		t.Fatalf("CollectDefinitions failed: %v", err)
	}
	printCodeElements(fCtx)

	// 4. 验证 Record 特性 (UserPoint)
	const pkg = "com.example.modern"
	// 4. 验证 Record 特性 (UserPoint)
	t.Run("Verify Record UserPoint QN", func(t *testing.T) {
		const recordQN = pkg + ".UserPoint"

		defs := fCtx.DefinitionsBySN["UserPoint"]
		require.NotEmpty(t, defs)
		assert.Equal(t, recordQN, defs[0].Element.QualifiedName)

		// 验证静态字段 ORIGIN
		originDef := fCtx.DefinitionsBySN["ORIGIN"]
		require.NotEmpty(t, originDef)
		assert.Equal(t, recordQN+".ORIGIN", originDef[0].Element.QualifiedName)

		// 验证 Record 组件 (x, y)
		// 注意：根据 collector 逻辑，组件会被同时识别为 Field 和 Method(Accessor)
		for _, comp := range []string{"x", "y"} {
			compDefs := fCtx.DefinitionsBySN[comp]
			require.NotEmpty(t, compDefs, "Component %q should be defined", comp)

			var hasField, hasMethod bool
			for _, d := range compDefs {
				if d.Element.Kind == model.Field {
					hasField = true
					// 字段 QN：com.example.modern.UserPoint.x
					assert.Equal(t, recordQN+"."+comp, d.Element.QualifiedName)
				}
				if d.Element.Kind == model.Method {
					hasMethod = true
					// 访问器方法 QN：com.example.modern.UserPoint.x()
					assert.Equal(t, recordQN+"."+comp+"()", d.Element.QualifiedName)
					// Record 方法的 IncludeParamNameQN 通常与 QN 一致（无参）
					assert.Equal(t, recordQN+"."+comp+"()", d.Element.Extra.MethodExtra.IncludeParamNameQN)
				}
			}
			assert.True(t, hasField, "Should have Field definition for %q", comp)
			assert.True(t, hasMethod, "Should have Method definition for %q", comp)
		}
	})

	// 5. 验证 Sealed Interface (Shape)
	t.Run("Verify Sealed Interface Shape QN", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN["Shape"]
		require.NotEmpty(t, defs)

		elem := defs[0].Element
		assert.Equal(t, pkg+".Shape", elem.QualifiedName)
		assert.Contains(t, elem.Extra.Modifiers, "sealed")
	})

	// 6. 验证实现类 (Circle)
	t.Run("Verify Final Class Circle QN", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN["Circle"]
		require.NotEmpty(t, defs)

		elem := defs[0].Element
		// 虽然 Circle 不是 public，但它在当前 package 下，QN 应包含完整包名
		assert.Equal(t, pkg+".Circle", elem.QualifiedName)
		assert.Contains(t, elem.Extra.Modifiers, "final")
	})

	fmt.Println("Java Collector Modern Java Features test completed successfully.")
}

// 验证顶层类
// 验证方法内部的局部类
// 验证局部类内部的方法
// 匿名内部类安全性验证
func TestJavaCollector_CallbackManager(t *testing.T) {
	// 1. 获取测试文件路径
	filePath := getTestFilePath(filepath.Join("com", "example", "service", "CallbackManager.java"))

	// 2. 解析源码
	rootNode, sourceBytes, err := getJavaParser(t).ParseFile(filePath, true, true)
	if err != nil {
		t.Fatalf("Failed to parse file: %v", err)
	}

	// 3. 运行 Collector
	collector := java.NewJavaCollector()
	fCtx, err := collector.CollectDefinitions(rootNode, filePath, sourceBytes)
	if err != nil {
		t.Fatalf("CollectDefinitions failed: %v", err)
	}
	printCodeElements(fCtx)

	const pkg = "com.example.service"
	const managerQN = pkg + ".CallbackManager"

	// 4. 验证顶层类
	t.Run("Verify Top Level Class", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN["CallbackManager"]
		require.NotEmpty(t, defs)
		assert.Equal(t, managerQN, defs[0].Element.QualifiedName)
	})

	// 5. 验证方法内部的局部类 (Local Class)
	t.Run("Verify Local Class QN", func(t *testing.T) {
		localClassName := "LocalValidator"
		defs := fCtx.DefinitionsBySN[localClassName]
		require.NotEmpty(t, defs)

		elem := defs[0].Element
		// 核心修正：register 方法无参，其 QN 为 register()
		// 因此局部类路径为：CallbackManager.register().LocalValidator
		expectedQN := managerQN + ".register().LocalValidator"
		assert.Equal(t, expectedQN, elem.QualifiedName, "Local class QN should follow method QN format")

		if elem.Kind != model.Class {
			t.Errorf("Expected Kind CLASS, got %s", elem.Kind)
		}
	})

	// 6. 验证局部类内部的方法 (带参数后缀校验)
	t.Run("Verify Method Inside Local Class QN", func(t *testing.T) {
		methodName := "isValid"
		defs := fCtx.DefinitionsBySN[methodName]
		require.NotEmpty(t, defs)

		elem := defs[0].Element
		me := elem.Extra.MethodExtra
		require.NotNil(t, me)

		// 预期: CallbackManager.register().LocalValidator.isValid()
		expectedQN := managerQN + ".register().LocalValidator.isValid()"
		assert.Equal(t, expectedQN, elem.QualifiedName, "Method inside local class QN mismatch")

		// 校验带参数名的 QN (由于无参，与 QN 一致)
		assert.Equal(t, expectedQN, me.IncludeParamNameQN)
	})

	// 7. 匿名内部类安全性与 register 方法校验
	t.Run("Verify Register Method and Anonymous Safety", func(t *testing.T) {
		// 校验外部 register 方法本身
		regDefs := fCtx.DefinitionsBySN["register"]
		require.NotEmpty(t, regDefs)
		assert.Equal(t, managerQN+".register()", regDefs[0].Element.QualifiedName)

		// 遍历检查，确保没有空的 QN 或损坏的路径
		for _, entries := range fCtx.DefinitionsBySN {
			for _, entry := range entries {
				qn := entry.Element.QualifiedName
				// 匿名类不应该有名称，因此不应出现在索引中
				assert.NotEmpty(t, entry.Element.Name)
				assert.NotContains(t, qn, "..")
			}
		}
	})
}

// 验证 Record 定义解析
// 验证 Record Components (Field + Method)
// 验证紧凑构造函数 (Compact Constructor)
// 验证显式定义的方法
// 验证变长参数 (Varargs) 类型处理
// 验证构造函数参数
// 验证深层嵌套（内部类方法）参数
func TestJavaCollector_ModernRecord(t *testing.T) {
	// 1. 获取测试文件路径
	filePath := getTestFilePath(filepath.Join("com", "example", "shop", "ModernOrderProcessor.java"))

	// 2. 解析源码
	rootNode, sourceBytes, err := getJavaParser(t).ParseFile(filePath, true, true)
	if err != nil {
		t.Fatalf("Failed to parse file: %v", err)
	}

	// 3. 运行 Collector
	collector := java.NewJavaCollector()
	fCtx, err := collector.CollectDefinitions(rootNode, filePath, sourceBytes)
	if err != nil {
		t.Fatalf("CollectDefinitions failed: %v", err)
	}
	printCodeElements(fCtx)

	const pkg = "com.example.shop"
	const recordQN = pkg + ".Order"

	// 4. 验证 Record 定义解析
	t.Run("Verify Record Declaration", func(t *testing.T) {
		orderDefs := fCtx.DefinitionsBySN["Order"]
		require.NotEmpty(t, orderDefs)

		orderElem := orderDefs[0].Element
		assert.Equal(t, model.Class, orderElem.Kind)
		assert.Equal(t, recordQN, orderElem.QualifiedName)
	})

	// 5. 验证 Record Components (Field + Method)
	t.Run("Verify Record Components and Implicit Accessors QN", func(t *testing.T) {
		priceDefs := fCtx.DefinitionsBySN["price"]
		// 应包含 Field 'price' 和 Method 'price()'
		assert.GreaterOrEqual(t, len(priceDefs), 2)

		var hasField, hasMethod bool
		for _, d := range priceDefs {
			if d.Element.Kind == model.Field {
				hasField = true
				// 字段 QN: com.example.shop.Order.price
				assert.Equal(t, recordQN+".price", d.Element.QualifiedName)
			}
			if d.Element.Kind == model.Method {
				hasMethod = true
				// 访问器方法 QN: com.example.shop.Order.price()
				assert.Equal(t, recordQN+".price()", d.Element.QualifiedName)
				// 校验 IncludeParamNameQN (无参)
				assert.Equal(t, recordQN+".price()", d.Element.Extra.MethodExtra.IncludeParamNameQN)
			}
		}
		assert.True(t, hasField)
		assert.True(t, hasMethod)
	})

	// 6. 验证紧凑构造函数 (Compact Constructor)
	t.Run("Verify Compact Constructor QN", func(t *testing.T) {
		constructorDefs := fCtx.DefinitionsBySN["Order"]
		found := false
		for _, d := range constructorDefs {
			// 紧凑构造函数对应 Record 完整参数列表
			// 期望 QN: com.example.shop.Order.Order(String,double)
			if d.Element.Kind == model.Method && strings.Contains(d.Element.QualifiedName, recordQN+".Order(") {
				found = true
				assert.Equal(t, recordQN+".Order(String,double)", d.Element.QualifiedName)
				assert.Equal(t, recordQN+".Order(String id, double price)", d.Element.Extra.MethodExtra.IncludeParamNameQN)
			}
		}
		assert.True(t, found, "Compact constructor should be indexed with erased types")
	})

	// 7. 验证显式定义的方法
	t.Run("Verify Explicit Methods QN", func(t *testing.T) {
		methods := []struct {
			name string
			qn   string
		}{
			{"process", recordQN + ".process()"},
			{"log", recordQN + ".log()"},
		}
		for _, m := range methods {
			defs := fCtx.DefinitionsBySN[m.name]
			require.NotEmpty(t, defs)
			assert.Equal(t, model.Method, defs[0].Element.Kind)
			assert.Equal(t, m.qn, defs[0].Element.QualifiedName)
		}
	})

	// 8. 修正后的参数作用域验证 (假设这些参数存在于 Order 的某个方法中)
	// 注意：此处修正了原测试用例中路径与文件名不符的问题
	t.Run("Verify Parameter Scopes in Order", func(t *testing.T) {
		// 验证 Record 组件作为参数（在构造函数中）
		idParamDefs := fCtx.DefinitionsBySN["id"]
		foundIdParam := false
		for _, d := range idParamDefs {
			// 参数 QN: 类.方法(参数).参数名
			if d.Element.Kind == model.Variable && strings.Contains(d.Element.QualifiedName, ".Order(String,double).id") {
				foundIdParam = true
				assert.Equal(t, recordQN+".Order(String,double).id", d.Element.QualifiedName)
			}
		}
		assert.True(t, foundIdParam, "Record component 'id' as constructor parameter not found")
	})
}

// 辅助函数
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
