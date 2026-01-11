package java_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CodMac/go-treesitter-dependency-analyzer/core"
	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	"github.com/CodMac/go-treesitter-dependency-analyzer/parser"
	"github.com/CodMac/go-treesitter-dependency-analyzer/x/java"

	_ "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

func getTestFilePath(name string) string {
	currentDir, _ := filepath.Abs(filepath.Dir("."))
	return filepath.Join(currentDir, "testdata", name)
}

// 将返回值类型改为接口 parser.Parser
func getJavaParser(t *testing.T) parser.Parser {
	javaParser, err := parser.NewParser(core.LangJava)
	if err != nil {
		t.Fatalf("Failed to create Java parser: %v", err)
	}

	return javaParser
}

const printEle = true

func printCodeElements(fCtx *core.FileContext) {
	if !printEle {
		return
	}

	fmt.Printf("Package: %s\n", fCtx.PackageName)
	for _, defs := range fCtx.DefinitionsBySN {
		for _, def := range defs {
			fmt.Printf("Short: %s -> Kind: %s, QN: %s\n", def.Element.Name, def.Element.Kind, def.Element.QualifiedName)
		}
	}
}

func TestJavaCollector_AbstractBaseEntity(t *testing.T) {
	// 1. 获取测试文件路径
	filePath := getTestFilePath(filepath.Join("com", "example", "base", "AbstractBaseEntity.java"))

	// 2. 解析源码
	rootNode, sourceBytes, err := getJavaParser(t).ParseFile(filePath, false, true)
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

	// 断言 1: 包名验证
	expectedPackage := "com.example.base"
	if fCtx.PackageName != expectedPackage {
		t.Errorf("Expected PackageName %q, got %q", expectedPackage, fCtx.PackageName)
	}

	// 断言 2: Imports 数量及内容验证
	t.Run("Verify Imports", func(t *testing.T) {
		expectedImports := []string{"java.io.Serializable", "java.util.Date"}
		if len(fCtx.Imports) != len(expectedImports) {
			t.Errorf("Expected %d imports, got %d", len(expectedImports), len(fCtx.Imports))
		}

		for _, path := range expectedImports {
			parts := strings.Split(path, ".")
			alias := parts[len(parts)-1]
			if imps, ok := fCtx.Imports[alias]; !ok || imps[0].RawImportPath != path {
				t.Errorf("Missing or incorrect import for %s", path)
			}
		}
	})

	// 断言 3: 类定义、QN、Kind、Abstract 属性、签名验证
	t.Run("Verify AbstractBaseEntity Class", func(t *testing.T) {
		qn := "com.example.base.AbstractBaseEntity"
		defs := findDefinitionsByQN(fCtx, qn)
		if len(defs) == 0 {
			t.Fatalf("Definition not found for QN: %s", qn)
		}

		elem := defs[0].Element
		if elem.Kind != model.Class {
			t.Errorf("Expected Kind CLASS, got %s", elem.Kind)
		}

		if isAbs, ok := elem.Extra.Mores[java.ClassIsAbstract].(bool); !ok || !isAbs {
			t.Error("Expected java.class.is_abstract to be true")
		}

		// 验证签名 (注意：由于 JavaCollector 内部实现可能不同，这里匹配核心部分)
		expectedSign := "public abstract class AbstractBaseEntity<ID> implements Serializable"
		if expectedSign != elem.Signature {
			t.Errorf("Signature mismatch. Got: %q, Expected: %s", elem.Signature, expectedSign)
		}
	})

	// 断言 4: 字段 id 验证
	t.Run("Verify Field id", func(t *testing.T) {
		qn := "com.example.base.AbstractBaseEntity.id"
		defs := findDefinitionsByQN(fCtx, qn)
		if len(defs) == 0 {
			t.Fatalf("Field id not found")
		}

		elem := defs[0].Element
		if elem.Kind != model.Field {
			t.Errorf("Expected Field, got %s", elem.Kind)
		}

		if tpe := elem.Extra.Mores[java.FieldType]; tpe != "ID" {
			t.Errorf("Expected type ID, got %v", tpe)
		}

		if !contains(elem.Extra.Modifiers, "protected") {
			t.Error("Modifiers should contain 'protected'")
		}
	})

	// 断言 5: 字段 createdAt 验证
	t.Run("Verify Field createdAt", func(t *testing.T) {
		qn := "com.example.base.AbstractBaseEntity.createdAt"
		defs := findDefinitionsByQN(fCtx, qn)
		if len(defs) == 0 {
			t.Fatalf("Field createdAt not found")
		}

		elem := defs[0].Element
		if tpe := elem.Extra.Mores[java.FieldType]; tpe != "Date" {
			t.Errorf("Expected type Date, got %v", tpe)
		}

		if !contains(elem.Extra.Modifiers, "private") {
			t.Error("Modifiers should contain 'private'")
		}
	})

	// 断言 6 & 7: 方法 getId 和 setId 验证
	t.Run("Verify Methods", func(t *testing.T) {
		// getId()
		getIdQN := "com.example.base.AbstractBaseEntity.getId()"
		getDefs := findDefinitionsByQN(fCtx, getIdQN)
		if len(getDefs) == 0 {
			t.Fatalf("Method getId() not found")
		}

		getElem := getDefs[0].Element
		if ret := getElem.Extra.Mores[java.MethodReturnType]; ret != "ID" {
			t.Errorf("getId expected return ID, got %v", ret)
		}

		// setId(ID id) - 验证 QN 括号内为类型
		setIdQN := "com.example.base.AbstractBaseEntity.setId(ID)"
		setDefs := findDefinitionsByQN(fCtx, setIdQN)
		if len(setDefs) == 0 {
			t.Fatalf("Method setId(ID) not found")
		}

		setElem := setDefs[0].Element
		if ret := setElem.Extra.Mores[java.MethodReturnType]; ret != "void" {
			t.Errorf("setId expected return void, got %v", ret)
		}
	})

	// 断言 8 & 9: 内部类 EntityMeta 及字段 tableName
	t.Run("Verify Nested Class EntityMeta", func(t *testing.T) {
		classQN := "com.example.base.AbstractBaseEntity.EntityMeta"
		classDefs := findDefinitionsByQN(fCtx, classQN)
		if len(classDefs) == 0 {
			t.Fatalf("Nested class EntityMeta not found")
		}

		classElem := classDefs[0].Element
		if !contains(classElem.Extra.Modifiers, "static") {
			t.Error("Should be static")
		}

		// 验证内部字段 tableName 的递归 QN
		fieldQN := "com.example.base.AbstractBaseEntity.EntityMeta.tableName"
		fieldDefs := findDefinitionsByQN(fCtx, fieldQN)
		if len(fieldDefs) == 0 {
			t.Fatalf("Field tableName not found in nested class")
		}

		fieldElem := fieldDefs[0].Element
		if tpe := fieldElem.Extra.Mores[java.FieldType]; tpe != "String" {
			t.Errorf("tableName expected String, got %v", tpe)
		}
	})
}

func TestJavaCollector_BaseClassHierarchy(t *testing.T) {
	// 1. 获取测试文件路径
	filePath := getTestFilePath(filepath.Join("com", "example", "base", "BaseClass.java"))

	// 2. 解析与收集
	rootNode, sourceBytes, err := getJavaParser(t).ParseFile(filePath, false, true)
	if err != nil {
		t.Fatalf("Failed to parse file: %v", err)
	}

	collector := java.NewJavaCollector()
	fCtx, err := collector.CollectDefinitions(rootNode, filePath, sourceBytes)
	if err != nil {
		t.Fatalf("CollectDefinitions failed: %v", err)
	}

	printCodeElements(fCtx)

	// 断言 1 & 2: 验证 BaseClass (Abstract, Annotations, Interfaces)
	t.Run("Verify BaseClass Metadata", func(t *testing.T) {
		qn := "com.example.base.BaseClass"
		defs := findDefinitionsByQN(fCtx, qn)
		if len(defs) == 0 {
			t.Fatalf("BaseClass not found")
		}
		elem := defs[0].Element

		// 断言 1: 注解验证
		expectedAnnos := []string{"@Deprecated", "@SuppressWarnings(\"unused\")"}
		for _, anno := range expectedAnnos {
			if !contains(elem.Extra.Annotations, anno) {
				t.Errorf("BaseClass missing annotation: %s", anno)
			}
		}

		// 断言 2: Abstract 属性与接口
		if isAbs, ok := elem.Extra.Mores[java.ClassIsAbstract].(bool); !ok || !isAbs {
			t.Error("Expected java.class.is_abstract to be true")
		}

		interfaces, ok := elem.Extra.Mores[java.ClassImplementedInterfaces].([]string)
		if !ok || !contains(interfaces, "Serializable") {
			t.Errorf("Expected Serializable interface, got %v", elem.Extra.Mores[java.ClassImplementedInterfaces])
		}
	})

	// 断言 3 & 4: 验证 FinalClass (Final, SuperClass, Multiple Interfaces, Location)
	t.Run("Verify FinalClass Metadata", func(t *testing.T) {
		qn := "com.example.base.FinalClass"
		defs := findDefinitionsByQN(fCtx, qn)
		if len(defs) == 0 {
			t.Fatalf("FinalClass not found")
		}
		elem := defs[0].Element

		// 断言 4: Kind 验证
		if elem.Kind != model.Class {
			t.Errorf("Expected Kind CLASS, got %s", elem.Kind)
		}

		// 断言 4: 位置信息验证 (FinalClass 在 BaseClass 之后，大致在第 11 行左右)
		if elem.Location.StartLine < 5 {
			t.Errorf("FinalClass StartLine seems incorrect: %d", elem.Location.StartLine)
		}

		// 断言 3: Final 属性
		if isFinal, ok := elem.Extra.Mores[java.ClassIsFinal].(bool); !ok || !isFinal {
			t.Error("Expected java.class.is_final to be true")
		}

		// 断言 3: 父类验证
		super, _ := elem.Extra.Mores[java.ClassSuperClass].(string)
		if !strings.Contains(super, "BaseClass") {
			t.Errorf("Expected super class BaseClass, got %q", super)
		}

		// 断言 3: 多接口验证
		interfaces, _ := elem.Extra.Mores[java.ClassImplementedInterfaces].([]string)
		if len(interfaces) < 2 || !contains(interfaces, "Cloneable") || !contains(interfaces, "Runnable") {
			t.Errorf("Expected multiple interfaces (Cloneable, Runnable), got %v", interfaces)
		}
	})

	// 断言 5: 验证 FinalClass.run() 函数的注解
	t.Run("Verify FinalClass.run() Annotations", func(t *testing.T) {
		qn := "com.example.base.FinalClass.run()"
		defs := findDefinitionsByQN(fCtx, qn)
		if len(defs) == 0 {
			t.Fatalf("Method run() not found")
		}
		elem := defs[0].Element

		if !contains(elem.Extra.Annotations, "@Override") {
			t.Error("Method run() missing @Override annotation")
		}
	})
}

func TestJavaCollector_CallbackManager(t *testing.T) {
	// 1. 获取测试文件路径
	filePath := getTestFilePath(filepath.Join("com", "example", "base", "CallbackManager.java"))

	// 2. 解析源码与运行 Collector
	rootNode, sourceBytes, err := getJavaParser(t).ParseFile(filePath, false, true)
	if err != nil {
		t.Fatalf("Failed to parse file: %v", err)
	}

	collector := java.NewJavaCollector()
	fCtx, err := collector.CollectDefinitions(rootNode, filePath, sourceBytes)
	if err != nil {
		t.Fatalf("CollectDefinitions failed: %v", err)
	}

	printCodeElements(fCtx)

	// 验证 1: 验证方法内部定义的局部类 LocalValidator
	t.Run("Verify Local Class", func(t *testing.T) {
		// 根据你的 Collector 实现，局部类应该在方法 QN 下
		qn := "com.example.base.CallbackManager.register().LocalValidator"
		defs := findDefinitionsByQN(fCtx, qn)
		if len(defs) == 0 {
			t.Fatalf("Local class LocalValidator not found at %s", qn)
		}

		elem := defs[0].Element
		if elem.Kind != model.Class {
			t.Errorf("Expected Kind CLASS, got %s", elem.Kind)
		}

		// 验证局部类内部的方法
		methodQN := qn + ".isValid()"
		methodDefs := findDefinitionsByQN(fCtx, methodQN)
		if len(methodDefs) == 0 {
			t.Errorf("Method isValid() not found in local class")
		}
		if methodDefs[0].Element.Extra.Mores[java.MethodParameters] != nil {
			t.Errorf("Method isValid() found params")
		}
	})

	// 验证 2: 验证变量 r
	t.Run("Verify Variable r", func(t *testing.T) {
		qn := "com.example.base.CallbackManager.register().r"
		defs := findDefinitionsByQN(fCtx, qn)
		if len(defs) == 0 {
			t.Fatalf("Variable r not found at %s", qn)
		}

		elem := defs[0].Element
		if tpe := elem.Extra.Mores[java.VariableType]; tpe != "Runnable" {
			t.Errorf("Expected type Runnable, got %v", tpe)
		}
	})

	// 验证 3: 验证匿名内部类及其方法 run()
	t.Run("Verify Anonymous Inner Class and Run Method", func(t *testing.T) {
		// 修正路径：anonymousClass$1 现在应该正确嵌套了 run()
		anonQN := "com.example.base.CallbackManager.register().anonymousClass$1"
		runQN := anonQN + ".run()"

		runDefs := findDefinitionsByQN(fCtx, runQN)
		if len(runDefs) == 0 {
			t.Fatalf("Method run() not found at expected QN: %s", runQN)
		}

		elem := runDefs[0].Element
		if !contains(elem.Extra.Annotations, "@Override") {
			t.Error("Method run() missing @Override")
		}
	})
}

func TestJavaCollector_ConfigService(t *testing.T) {
	// 1. 获取测试文件路径
	filePath := getTestFilePath(filepath.Join("com", "example", "base", "ConfigService.java"))

	// 2. 解析源码与运行 Collector
	rootNode, sourceBytes, err := getJavaParser(t).ParseFile(filePath, false, false)
	if err != nil {
		t.Fatalf("Failed to parse file: %v", err)
	}

	collector := java.NewJavaCollector()
	fCtx, err := collector.CollectDefinitions(rootNode, filePath, sourceBytes)
	if err != nil {
		t.Fatalf("CollectDefinitions failed: %v", err)
	}

	printCodeElements(fCtx)

	// 验证 1: 变长参数 (Object...) 与 数组参数 (String[])
	t.Run("Verify Variadic and Array Parameters", func(t *testing.T) {
		// 注意：QN 内部的参数类型应反映原始定义
		qn := "com.example.base.ConfigService.updateConfigs(String[],Object...)"
		defs := findDefinitionsByQN(fCtx, qn)
		if len(defs) == 0 {
			t.Fatalf("Method updateConfigs not found with expected signature QN: %s", qn)
		}

		elem := defs[0].Element
		params, ok := elem.Extra.Mores[java.MethodParameters].([]string)
		if !ok || len(params) != 2 {
			t.Fatalf("Expected 2 parameters, got %v", params)
		}

		// 验证数组参数
		if !strings.Contains(params[0], "String[]") {
			t.Errorf("Expected first param to be String[], got %s", params[0])
		}

		// 验证变长参数
		if !strings.Contains(params[1], "Object...") {
			t.Errorf("Expected second param to be Object..., got %s", params[1])
		}
	})

	// 验证 2: 复杂多属性注解
	t.Run("Verify Complex Annotations", func(t *testing.T) {
		qn := "com.example.base.ConfigService.legacyMethod()"
		defs := findDefinitionsByQN(fCtx, qn)
		if len(defs) == 0 {
			t.Fatalf("Method legacyMethod not found")
		}

		elem := defs[0].Element
		annos := elem.Extra.Annotations

		// 验证 @SuppressWarnings 的数组格式
		foundSuppressed := false
		for _, a := range annos {
			if strings.Contains(a, "@SuppressWarnings") && strings.Contains(a, "\"unchecked\"") && strings.Contains(a, "\"rawtypes\"") {
				foundSuppressed = true
				break
			}
		}
		if !foundSuppressed {
			t.Errorf("Could not find complete @SuppressWarnings annotation, got: %v", annos)
		}

		// 验证 @Deprecated 的多属性 (since, forRemoval)
		foundDeprecated := false
		for _, a := range annos {
			if strings.Contains(a, "@Deprecated") && strings.Contains(a, "since = \"1.2\"") && strings.Contains(a, "forRemoval = true") {
				foundDeprecated = true
				break
			}
		}
		if !foundDeprecated {
			t.Errorf("Could not find detailed @Deprecated annotation, got: %v", annos)
		}
	})

	t.Run("Verify Specific Parameters", func(t *testing.T) {
		// 验证 keys
		keysQN := "com.example.base.ConfigService.updateConfigs(String[],Object...).keys"
		if len(findDefinitionsByQN(fCtx, keysQN)) == 0 {
			t.Errorf("Variable 'keys' not found")
		}

		// 验证 values
		valuesQN := "com.example.base.ConfigService.updateConfigs(String[],Object...).values"
		vDefs := findDefinitionsByQN(fCtx, valuesQN)
		if len(vDefs) == 0 {
			t.Fatalf("Variable 'values' not found")
		}

		vElem := vDefs[0].Element
		if tpe := vElem.Extra.Mores[java.VariableType]; tpe != "Object..." {
			t.Errorf("Expected type Object..., got %v", tpe)
		}
	})
}

func TestJavaCollector_DataProcessor(t *testing.T) {
	// 1. 获取测试文件路径
	filePath := getTestFilePath(filepath.Join("com", "example", "base", "DataProcessor.java"))

	// 2. 解析源码与运行 Collector
	rootNode, sourceBytes, err := getJavaParser(t).ParseFile(filePath, false, false)
	if err != nil {
		t.Fatalf("Failed to parse file: %v", err)
	}

	collector := java.NewJavaCollector()
	fCtx, err := collector.CollectDefinitions(rootNode, filePath, sourceBytes)
	if err != nil {
		t.Fatalf("CollectDefinitions failed: %v", err)
	}

	printCodeElements(fCtx)

	// 验证 1: 接口定义、多继承与泛型
	t.Run("Verify Interface Heritage and Generics", func(t *testing.T) {
		qn := "com.example.base.DataProcessor"
		defs := findDefinitionsByQN(fCtx, qn)
		if len(defs) == 0 {
			t.Fatalf("Interface DataProcessor not found")
		}

		elem := defs[0].Element
		// 验证接口继承
		ifaces, _ := elem.Extra.Mores[java.InterfaceImplementedInterfaces].([]string)
		expectedIfaces := []string{"Runnable", "AutoCloseable"}
		for _, expected := range expectedIfaces {
			if !contains(ifaces, expected) {
				t.Errorf("Expected interface %s not found in %v", expected, ifaces)
			}
		}

		// 验证签名中的泛型参数 (T extends AbstractBaseEntity<?>)
		if !strings.Contains(elem.Signature, "<T extends AbstractBaseEntity<?>>") {
			t.Errorf("Signature missing generics: %s", elem.Signature)
		}
	})

	// 验证 2: 方法的 Throws 异常
	t.Run("Verify Method Throws", func(t *testing.T) {
		// 注意：泛型 T 在 QN 中按原样提取
		qn := "com.example.base.DataProcessor.processAll(String)"
		defs := findDefinitionsByQN(fCtx, qn)
		if len(defs) == 0 {
			t.Fatalf("Method processAll not found")
		}

		elem := defs[0].Element
		throws, _ := elem.Extra.Mores[java.MethodThrowsTypes].([]string)

		expectedThrows := []string{"RuntimeException", "Exception"}
		if len(throws) != 2 {
			t.Fatalf("Expected 2 throws types, got %v", throws)
		}
		for i, e := range expectedThrows {
			if throws[i] != e {
				t.Errorf("Expected throw %s, got %s", e, throws[i])
			}
		}
	})

	// 验证 3: Java 8 Default 方法修饰符
	t.Run("Verify Default Method", func(t *testing.T) {
		qn := "com.example.base.DataProcessor.stop()"
		defs := findDefinitionsByQN(fCtx, qn)
		if len(defs) == 0 {
			t.Fatalf("Method stop not found")
		}

		elem := defs[0].Element
		// 验证是否包含 default 关键字
		if !contains(elem.Extra.Modifiers, "default") {
			t.Errorf("Method stop should have 'default' modifier, got %v", elem.Extra.Modifiers)
		}

		// 验证 Signature 是否正确包含 default
		if !strings.HasPrefix(elem.Signature, "default void stop()") {
			t.Errorf("Signature prefix incorrect: %s", elem.Signature)
		}
	})
}

func TestJavaCollector_NestedAndStaticBlocks(t *testing.T) {
	filePath := getTestFilePath(filepath.Join("com", "example", "base", "OuterClass.java"))
	rootNode, sourceBytes, err := getJavaParser(t).ParseFile(filePath, true, true)
	if err != nil {
		t.Fatalf("Failed to parse file: %v", err)
	}

	collector := java.NewJavaCollector()
	fCtx, err := collector.CollectDefinitions(rootNode, filePath, sourceBytes)
	if err != nil {
		t.Fatalf("CollectDefinitions failed: %v", err)
	}

	printCodeElements(fCtx)

	// 验证 1: 静态初始化块与实例块
	t.Run("Verify Initialization Blocks", func(t *testing.T) {
		// 静态块通常被识别为 static_initializer 节点, 我们将其命名为 $static
		staticBlockQN := "com.example.base.OuterClass.$static$1"
		if len(findDefinitionsByQN(fCtx, staticBlockQN)) == 0 {
			t.Errorf("Static initializer block not found at expected QN: %s", staticBlockQN)
		}
	})

	// 验证 2: 内部类与静态嵌套类
	t.Run("Verify Nested Classes", func(t *testing.T) {
		// 内部类 QN
		innerQN := "com.example.base.OuterClass.InnerClass"
		if len(findDefinitionsByQN(fCtx, innerQN)) == 0 {
			t.Errorf("InnerClass not found")
		}

		// 静态嵌套类方法 QN
		nestedMethodQN := "com.example.base.OuterClass.StaticNestedClass.run()"
		if len(findDefinitionsByQN(fCtx, nestedMethodQN)) == 0 {
			t.Errorf("Method run() in StaticNestedClass not found")
		}
	})

	// 验证 3: 方法内部类 (Local Class)
	t.Run("Verify Local Class", func(t *testing.T) {
		// 注意层级：OuterClass -> scopeTest() -> LocalClass
		localClassQN := "com.example.base.OuterClass.scopeTest().LocalClass"
		defs := findDefinitionsByQN(fCtx, localClassQN)
		if len(defs) == 0 {
			t.Errorf("Local class inside method not found at: %s", localClassQN)
		}
	})
}

// 辅助函数：根据 QN 在 fCtx 中查找定义
func findDefinitionsByQN(fCtx *core.FileContext, targetQN string) []*core.DefinitionEntry {
	var result []*core.DefinitionEntry
	for _, entries := range fCtx.DefinitionsBySN {
		for _, entry := range entries {
			if entry.Element.QualifiedName == targetQN {
				result = append(result, entry)
			}
		}
	}

	return result
}

// 辅助函数：判断 slice 是否包含 string
func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}
