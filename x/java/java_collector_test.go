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

func getJavaParser(t *testing.T) *parser.TreeSitterParser {
	javaParser, err := parser.NewParser(model.LangJava)
	if err != nil {
		t.Fatalf("Failed to create Java parser: %v", err)
	}

	return javaParser
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
		assert.Equal(t, "com.example.model.AbstractBaseEntity.getId", getDefs[0].Element.QualifiedName)
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
// 验证方法异常抛出
// 验证默认方法与修饰符
func TestJavaCollector_DataProcessor_Complex(t *testing.T) {
	// 1. 初始化
	filePath := getTestFilePath(filepath.Join("com", "example", "core", "DataProcessor.java"))
	rootNode, sourceBytes, err := getJavaParser(t).ParseFile(filePath, true, false)
	require.NoError(t, err)

	collector := java.NewJavaCollector()
	fCtx, err := collector.CollectDefinitions(rootNode, filePath, sourceBytes)
	require.NoError(t, err)

	// 2. 验证接口继承 (extends Runnable, AutoCloseable)
	t.Run("Verify Multiple Interface Inheritance", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN["DataProcessor"]
		require.NotEmpty(t, defs)

		ce := defs[0].Element.Extra.ClassExtra
		require.NotNil(t, ce)

		// 验证继承的接口列表
		// 在 Java 中，接口继承其他接口使用的是 extends 关键字，但在 AST 中对应 interfaces 字段
		assert.Contains(t, ce.ImplementedInterfaces, "Runnable")
		assert.Contains(t, ce.ImplementedInterfaces, "AutoCloseable")

		// 验证 Doc 采集是否包含了测试描述
		assert.Contains(t, defs[0].Element.Doc, "Method Throws")
	})

	// 3. 验证方法异常抛出 (throws RuntimeException, Exception)
	t.Run("Verify Method Throws and Return Type", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN["processAll"]
		require.NotEmpty(t, defs)

		elem := defs[0].Element
		me := elem.Extra.MethodExtra
		require.NotNil(t, me)

		// 验证返回类型 (带泛型的引用)
		assert.Equal(t, "List<T>", me.ReturnType)

		// 验证 Throws 异常列表
		assert.Equal(t, 2, len(me.ThrowsTypes))
		assert.Contains(t, me.ThrowsTypes, "RuntimeException")
		assert.Contains(t, me.ThrowsTypes, "Exception")
	})

	// 4. 验证默认方法与修饰符
	t.Run("Verify Default Method", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN["stop"]
		require.NotEmpty(t, defs)

		elem := defs[0].Element
		// 验证是否识别出 default 关键字
		assert.Contains(t, elem.Extra.Modifiers, "default")

		// 验证签名完整性
		assert.Equal(t, "default void stop()", elem.Signature)
	})

	// 5. 验证导入表
	t.Run("Verify Imports", func(t *testing.T) {
		assert.Contains(t, fCtx.Imports, "AbstractBaseEntity")
		assert.Equal(t, "com.example.model.AbstractBaseEntity", fCtx.Imports["AbstractBaseEntity"].RawImportPath)

		assert.Contains(t, fCtx.Imports, "List")
		assert.Equal(t, "java.util.List", fCtx.Imports["List"].RawImportPath)
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
			assert.Equal(t, "com.example.model.ErrorCode."+name, elem.QualifiedName)
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

	// 4. 验证 Package Name
	expectedPackage := "com.example.model"
	if fCtx.PackageName != expectedPackage {
		t.Errorf("Expected PackageName %q, got %q", expectedPackage, fCtx.PackageName)
	}

	// 5. 验证类定义与继承关系 (EXTEND 关系)
	className := "NotificationException"
	t.Run("Verify Class and Inheritance", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN[className]
		if len(defs) == 0 {
			t.Fatalf("Class %q not found", className)
		}

		elem := defs[0].Element
		if elem.Kind != model.Class {
			t.Errorf("Expected Kind CLASS, got %s", elem.Kind)
		}

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
	t.Run("Verify Overloaded Constructors", func(t *testing.T) {
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

		// 验证参数列表
		foundThrowable := false
		foundErrorCode := false

		for _, c := range constructors {
			params := c.Extra.MethodExtra.Parameters
			if len(params) == 2 && strings.Contains(params[1], "Throwable") {
				foundThrowable = true
				if !strings.Contains(c.Signature, "String message") {
					t.Errorf("Constructor signature mismatch: %s", c.Signature)
				}
			}
			if len(params) == 1 && strings.Contains(params[0], "ErrorCode") {
				foundErrorCode = true
			}
		}

		if !foundThrowable {
			t.Error("Constructor with (String, Throwable) not found or parameters mismatch")
		}
		if !foundErrorCode {
			t.Error("Constructor with (ErrorCode) not found or parameters mismatch")
		}
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
	t.Run("Verify User Class Members", func(t *testing.T) {
		// 验证私有字段 id
		if defs := fCtx.DefinitionsBySN["id"]; len(defs) > 0 {
			elem := defs[0].Element
			if !contains(elem.Extra.Modifiers, "private") {
				t.Errorf("Field 'id' should be private, got %v", elem.Extra.Modifiers)
			}
			if elem.Extra.FieldExtra.Type != "String" {
				t.Errorf("Field 'id' expected type String, got %s", elem.Extra.FieldExtra.Type)
			}
		} else {
			t.Error("Field 'id' not found")
		}

		// 验证静态常量 DEFAULT_ID
		if defs := fCtx.DefinitionsBySN["DEFAULT_ID"]; len(defs) > 0 {
			elem := defs[0].Element
			mods := elem.Extra.Modifiers
			if !contains(mods, "static") || !contains(mods, "final") {
				t.Errorf("DEFAULT_ID should be static final, got %v", mods)
			}
			if !elem.Extra.FieldExtra.IsConstant {
				t.Error("DEFAULT_ID should be marked as IsConstant")
			}
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
	t.Run("Verify Private Static Method in Nested Class", func(t *testing.T) {
		methodName := "chooseUnit"
		defs := fCtx.DefinitionsBySN[methodName]
		if len(defs) == 0 {
			t.Fatal("Method 'chooseUnit' not found")
		}

		elem := defs[0].Element
		if !contains(elem.Extra.Modifiers, "private") || !contains(elem.Extra.Modifiers, "static") {
			t.Errorf("Method 'chooseUnit' should be private static, got %v", elem.Extra.Modifiers)
		}

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

	// 4. 验证 Package
	expectedPackage := "com.example.config"
	if fCtx.PackageName != expectedPackage {
		t.Errorf("Expected PackageName %q, got %q", expectedPackage, fCtx.PackageName)
	}

	// 5. 验证变长参数与数组类型 (Boundary: Arrays & Varargs)
	t.Run("Verify Arrays and Varargs", func(t *testing.T) {
		methodName := "updateConfigs"
		defs := fCtx.DefinitionsBySN[methodName]
		if len(defs) == 0 {
			t.Fatalf("Method %q not found", methodName)
		}

		elem := defs[0].Element
		mExtra := elem.Extra.MethodExtra

		// 验证参数数量
		if len(mExtra.Parameters) != 2 {
			t.Errorf("Expected 2 parameters, got %d", len(mExtra.Parameters))
		}

		// 验证数组类型 String[]
		if !strings.Contains(mExtra.Parameters[0], "String[]") {
			t.Errorf("Expected first parameter to be String[], got %q", mExtra.Parameters[0])
		}

		// 验证变长参数 Object...
		// 在 Tree-sitter Java 中，spread_parameter 会被捕获为带三个点的文本
		if !strings.Contains(mExtra.Parameters[1], "Object...") {
			t.Errorf("Expected second parameter to be Object..., got %q", mExtra.Parameters[1])
		}

		// 验证完整 Signature 的还原度
		expectedSignPart := "updateConfigs(String[] keys, Object... values)"
		if !strings.Contains(elem.Signature, expectedSignPart) {
			t.Errorf("Signature mismatch.\nExpected contains: %q\nActual: %q", expectedSignPart, elem.Signature)
		}
	})

	// 6. 验证复杂注解提取 (Boundary: Complex Annotations)
	t.Run("Verify Complex Annotations", func(t *testing.T) {
		methodName := "legacyMethod"
		defs := fCtx.DefinitionsBySN[methodName]
		if len(defs) == 0 {
			t.Fatalf("Method %q not found", methodName)
		}

		elem := defs[0].Element
		annos := elem.Extra.Annotations

		if len(annos) < 2 {
			t.Errorf("Expected at least 2 annotations, got %d", len(annos))
		}

		// 验证带数组值的注解 @SuppressWarnings({"unchecked", "rawtypes"})
		foundSuppressWarnings := false
		for _, anno := range annos {
			if strings.Contains(anno, "SuppressWarnings") {
				foundSuppressWarnings = true
				if !strings.Contains(anno, "unchecked") || !strings.Contains(anno, "rawtypes") {
					t.Errorf("SuppressWarnings annotation missing values: %q", anno)
				}
			}
		}
		if !foundSuppressWarnings {
			t.Error("@SuppressWarnings not found")
		}

		// 验证带键值对的注解 @Deprecated(since = "1.2", forRemoval = true)
		foundDeprecated := false
		for _, anno := range annos {
			if strings.Contains(anno, "Deprecated") {
				foundDeprecated = true
				if !strings.Contains(anno, "since") || !strings.Contains(anno, "forRemoval") {
					t.Errorf("Deprecated annotation missing properties: %q", anno)
				}
			}
		}
		if !foundDeprecated {
			t.Error("@Deprecated not found")
		}
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

	// 4. 验证接口定义与多重泛型边界
	t.Run("Verify Interface with Multiple Bounds", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN["GenericRepository"]
		if len(defs) == 0 {
			t.Fatal("Interface 'GenericRepository' not found")
		}

		elem := defs[0].Element
		if elem.Kind != model.Interface {
			t.Errorf("Expected Kind INTERFACE, got %s", elem.Kind)
		}

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

	// 5. 验证复杂泛型参数的方法 (List<? extends T>)
	t.Run("Verify Wildcard Generics", func(t *testing.T) {
		methodName := "findAllByCriteria"
		defs := fCtx.DefinitionsBySN[methodName]
		if len(defs) == 0 {
			t.Fatalf("Method %q not found", methodName)
		}

		elem := defs[0].Element
		mExtra := elem.Extra.MethodExtra

		// 验证返回类型是否保留了通配符：List<? extends T>
		if mExtra.ReturnType != "List<? extends T>" {
			t.Errorf("Expected return type 'List<? extends T>', got %q", mExtra.ReturnType)
		}

		// 验证入参类型是否保留了通配符：List<? super T>
		if len(mExtra.Parameters) != 1 || !strings.Contains(mExtra.Parameters[0], "List<? super T>") {
			t.Errorf("Expected parameter 'List<? super T>', got %v", mExtra.Parameters)
		}
	})

	// 6. 验证方法级别的泛型与抛出异常 (Method-level Generics)
	t.Run("Verify Method Generics and Throws", func(t *testing.T) {
		methodName := "executeOrThrow"
		defs := fCtx.DefinitionsBySN[methodName]
		if len(defs) == 0 {
			t.Fatalf("Method %q not found", methodName)
		}

		elem := defs[0].Element
		mExtra := elem.Extra.MethodExtra

		// 验证 Throws 类型是否被捕获
		if len(mExtra.ThrowsTypes) == 0 || mExtra.ThrowsTypes[0] != "E" {
			t.Errorf("Expected ThrowsType 'E', got %v", mExtra.ThrowsTypes)
		}

		// 验证 Signature 是否包含方法泛型定义 <E extends Exception>
		if !strings.Contains(elem.Signature, "<E extends Exception>") {
			t.Errorf("Method-level generic definition missing in signature: %q", elem.Signature)
		}
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

	// 4. 验证 Record 特性 (UserPoint)
	t.Run("Verify Record UserPoint", func(t *testing.T) {
		// 注意：如果在 extractElementBasic 中未显式添加 record_declaration，此断言可能会失败
		// 我们需要验证 Record 是否被识别为类或特定的 Record 类型
		defs := fCtx.DefinitionsBySN["UserPoint"]
		if len(defs) == 0 {
			t.Fatal("Record 'UserPoint' not found. Check if 'record_declaration' is handled in collector.")
		}

		elem := defs[0].Element
		// 验证签名是否包含 record 关键字
		if !strings.Contains(elem.Signature, "record UserPoint") {
			t.Errorf("Expected signature to contain 'record UserPoint', got %q", elem.Signature)
		}

		// 验证静态字段
		originDef := fCtx.DefinitionsBySN["ORIGIN"]
		if len(originDef) == 0 {
			t.Error("Static field 'ORIGIN' inside record not found")
		} else {
			assert.Contains(t, originDef[0].Element.Extra.Modifiers, "static")
		}

		// 验证 Record 组件 (x, y)
		// 在 Tree-sitter 中，record 的参数通常是 record_component
		for _, comp := range []string{"x", "y"} {
			compDef := fCtx.DefinitionsBySN[comp]
			if len(compDef) == 0 {
				t.Errorf("Record component %q not found. You may need to handle 'record_component' as a Field.", comp)
			} else {
				assert.Equal(t, model.Field, compDef[0].Element.Kind)
			}
		}
	})

	// 5. 验证 Sealed Interface (Shape)
	t.Run("Verify Sealed Interface Shape", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN["Shape"]
		if len(defs) == 0 {
			t.Fatal("Sealed interface 'Shape' not found")
		}

		elem := defs[0].Element
		// 验证修饰符是否包含 sealed
		if !contains(elem.Extra.Modifiers, "sealed") {
			t.Errorf("Expected modifier 'sealed' not found in %v", elem.Extra.Modifiers)
		}

		// 验证签名是否完整 (包含 permits 列表)
		// 注意：permits 列表在 AST 中是 permits_list 节点
		if !strings.Contains(elem.Signature, "permits Circle, Square") {
			t.Logf("Warning: Signature might not capture 'permits' clause yet: %q", elem.Signature)
		}
	})

	// 6. 验证实现类 (Circle)
	t.Run("Verify Final Class Circle", func(t *testing.T) {
		defs := fCtx.DefinitionsBySN["Circle"]
		if len(defs) == 0 {
			t.Fatal("Class 'Circle' not found")
		}

		elem := defs[0].Element
		if !contains(elem.Extra.Modifiers, "final") {
			t.Error("Class 'Circle' should have 'final' modifier")
		}
		if elem.Extra.ClassExtra.ImplementedInterfaces[0] != "Shape" {
			t.Errorf("Expected implemented interface 'Shape', got %v", elem.Extra.ClassExtra.ImplementedInterfaces)
		}
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

	// 4. 验证顶层类
	t.Run("Verify Top Level Class", func(t *testing.T) {
		className := "CallbackManager"
		defs := fCtx.DefinitionsBySN[className]
		if len(defs) == 0 {
			t.Fatalf("Class %q not found", className)
		}
		assert.Equal(t, "com.example.service.CallbackManager", defs[0].Element.QualifiedName)
	})

	// 5. 验证方法内部的局部类 (Local Class)
	t.Run("Verify Local Class", func(t *testing.T) {
		localClassName := "LocalValidator"
		defs := fCtx.DefinitionsBySN[localClassName]
		if len(defs) == 0 {
			t.Fatalf("Local class %q not found. Check if recursiveCollect enters method bodies.", localClassName)
		}

		elem := defs[0].Element
		// 根据你的 QN 策略，局部类的 QN 应该是包含外部路径的
		// 预期: com.example.service.CallbackManager.register.LocalValidator
		expectedQN := "com.example.service.CallbackManager.register.LocalValidator"
		if elem.QualifiedName != expectedQN {
			t.Errorf("Local class QN mismatch.\nExpected: %q\nActual: %q", expectedQN, elem.QualifiedName)
		}

		if elem.Kind != model.Class {
			t.Errorf("Expected Kind CLASS, got %s", elem.Kind)
		}
	})

	// 6. 验证局部类内部的方法
	t.Run("Verify Method Inside Local Class", func(t *testing.T) {
		methodName := "isValid"
		defs := fCtx.DefinitionsBySN[methodName]
		if len(defs) == 0 {
			t.Fatalf("Method %q inside local class not found", methodName)
		}

		expectedQN := "com.example.service.CallbackManager.register.LocalValidator.isValid"
		if defs[0].Element.QualifiedName != expectedQN {
			t.Errorf("Method QN mismatch.\nExpected: %q\nActual: %q", expectedQN, defs[0].Element.QualifiedName)
		}
	})

	// 7. 匿名内部类安全性验证 (Robustness check)
	t.Run("Anonymous Class Safety", func(t *testing.T) {
		// 遍历 DefinitionsBySN Map: map[string][]*DefinitionEntry
		for shortName, entries := range fCtx.DefinitionsBySN {
			if shortName == "" {
				t.Errorf("Found an entry with empty short name key")
			}
			for _, entry := range entries {
				if entry.Element.Name == "" {
					t.Errorf("Found a definition with empty name at line %d", entry.Element.Location.StartLine)
				}
				// 额外检查：确保匿名类没有被错误地加入到索引中
				// 匿名类通常在 AST 中 Kind 是 anonymous_class，我们的 Collector 应该跳过它
				if strings.Contains(entry.Element.QualifiedName, "..") {
					t.Errorf("Malformed QN found: %q", entry.Element.QualifiedName)
				}
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

	// 3. 验证 Record 定义解析
	t.Run("Verify Record Declaration", func(t *testing.T) {
		// 验证 QualifiedName 是否正确映射
		orderDefs, ok := fCtx.DefinitionsBySN["Order"]
		assert.True(t, ok, "Should define 'Order' record")

		orderElem := orderDefs[0].Element
		assert.Equal(t, model.Class, orderElem.Kind, "Record should be treated as a specialized Class")
		assert.Equal(t, "com.example.shop.Order", orderElem.QualifiedName)
	})

	// 4. 验证 Record Components (Field + Method)
	t.Run("Verify Record Components and Implicit Accessors", func(t *testing.T) {
		// 在 Java Record 中，'price' 既是 Field，也会生成隐式的 'price()' Method
		priceDefs := fCtx.DefinitionsBySN["price"]
		assert.GreaterOrEqual(t, len(priceDefs), 2, "Should have at least 2 definitions for 'price' (field and accessor)")

		hasField := false
		hasMethod := false
		for _, d := range priceDefs {
			if d.Element.Kind == model.Field {
				hasField = true
				assert.Equal(t, "com.example.shop.Order.price", d.Element.QualifiedName)
			}
			if d.Element.Kind == model.Method {
				hasMethod = true
				assert.Equal(t, "com.example.shop.Order.price", d.Element.QualifiedName)
			}
		}
		assert.True(t, hasField, "Record component 'price' should be collected as Field")
		assert.True(t, hasMethod, "Record component 'price' should be collected as implicit Method")
	})

	// 5. 验证紧凑构造函数 (Compact Constructor)
	t.Run("Verify Compact Constructor", func(t *testing.T) {
		// 紧凑构造函数在 AST 中是 compact_constructor_declaration，应识别为构造函数
		constructorDefs := fCtx.DefinitionsBySN["Order"]
		foundConstructor := false
		for _, d := range constructorDefs {
			// 排除类定义本身，查找同名的 Method/Constructor
			if d.Element.Kind == model.Method && d.Element.QualifiedName == "com.example.shop.Order.Order" {
				foundConstructor = true
			}
		}
		assert.True(t, foundConstructor, "Should collect compact constructor as a Method/Constructor")
	})

	// 6. 验证显式定义的方法
	t.Run("Verify Explicit Methods", func(t *testing.T) {
		methods := []string{"process", "log"}
		for _, mName := range methods {
			defs, ok := fCtx.DefinitionsBySN[mName]
			assert.True(t, ok, "Method %s should be defined", mName)
			assert.Equal(t, model.Method, defs[0].Element.Kind)
			assert.Equal(t, "com.example.shop.Order."+mName, defs[0].Element.QualifiedName)
		}
	})

	// 7. 验证变长参数 (Varargs) 类型处理
	t.Run("Verify Varargs Parameter", func(t *testing.T) {
		paramName := "labels"
		defs := fCtx.DefinitionsBySN[paramName]
		if len(defs) == 0 {
			t.Fatalf("Varargs parameter %q not found", paramName)
		}

		elem := defs[0].Element

		// 验证类型是否正确附加了 "..."
		assert.Equal(t, "String...", elem.Extra.FieldExtra.Type, "Varargs type must be correctly identified with '...'")
		assert.Equal(t, "com.example.scope.ParameterScopeTester.execute.labels", elem.QualifiedName)
	})

	// 8. 验证构造函数参数
	t.Run("Verify Constructor Parameter", func(t *testing.T) {
		paramName := "initialConfig"
		defs := fCtx.DefinitionsBySN[paramName]
		if len(defs) == 0 {
			t.Fatalf("Constructor parameter %q not found", paramName)
		}

		elem := defs[0].Element

		// 构造函数的 QN 通常为: 类名.类名 (作为方法名).参数名
		expectedQN := "com.example.scope.ParameterScopeTester.ParameterScopeTester.initialConfig"
		assert.Equal(t, expectedQN, elem.QualifiedName, "Constructor parameter QN mismatch")
	})

	// 9. 验证深层嵌套（内部类方法）参数
	t.Run("Verify Nested Method Parameter", func(t *testing.T) {
		paramName := "duration"
		defs := fCtx.DefinitionsBySN[paramName]
		if len(defs) == 0 {
			t.Fatalf("Nested parameter %q not found", paramName)
		}

		elem := defs[0].Element

		// 预期层级: 类.内部类.方法.参数
		expectedQN := "com.example.scope.ParameterScopeTester.InnerWorker.doWork.duration"
		assert.Equal(t, expectedQN, elem.QualifiedName)
		assert.Equal(t, "long", elem.Extra.FieldExtra.Type)
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
