package java_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CodMac/go-treesitter-dependency-analyzer/model"
	"github.com/CodMac/go-treesitter-dependency-analyzer/parser"
	"github.com/CodMac/go-treesitter-dependency-analyzer/x/java" // 触发 init() 注册
	"github.com/stretchr/testify/assert"
)

// 辅助函数：解析并收集定义 (Phase 1)
func runPhase1Collection(t *testing.T, files []string) *model.GlobalContext {
	gc := model.NewGlobalContext()
	javaParser, err := parser.NewParser(model.LangJava)
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}
	defer javaParser.Close()

	col := java.NewJavaCollector()

	for _, file := range files {
		rootNode, sourceBytes, err := javaParser.ParseFile(file, true, false)
		if err != nil {
			t.Fatalf("Failed to parse file %s: %v", file, err)
		}

		fCtx, err := col.CollectDefinitions(rootNode, file, sourceBytes)
		if err != nil {
			t.Fatalf("Failed to collect definitions for %s: %v", file, err)
		}
		gc.RegisterFileContext(fCtx)
	}
	return gc
}

const printRel = true

func printRelations(relations []*model.DependencyRelation) {
	if !printRel {
		return
	}

	fmt.Printf("Found relation: [DependencyType] -> source(Kind::QualifiedName)==>target(Kind::QualifiedName)\n")
	for _, rel := range relations {
		fmt.Printf("Found relation: [%s] -> source(%s::%s)==>target(%s::%s)\n", rel.Type, rel.Source.Kind, rel.Source.QualifiedName, rel.Target.Kind, rel.Target.QualifiedName)
	}
}

// 验证结构关系：CONTAIN
// 验证行为关系：CREATE（由匿名内部类产生）、Call（由匿名内部类产生）
// 验证 JDK 内置符号解析
func TestJavaExtractor_CallbackManager(t *testing.T) {
	// 1. 准备测试文件路径
	targetFile := getTestFilePath(filepath.Join("com", "example", "service", "CallbackManager.java"))

	// 2. Phase 1: 构建全局上下文 (运行 Collector)
	// 即使只有一个文件，也需要运行 Phase 1 以便 Extractor 获取 FileContext
	gCtx := runPhase1Collection(t, []string{targetFile})

	// 3. Phase 2: 运行增强后的 Extractor
	ext := java.NewJavaExtractor()
	relations, err := ext.Extract(targetFile, gCtx)
	if err != nil {
		t.Fatalf("Extraction failed: %v", err)
	}
	printRelations(relations)

	// 4. 验证顶层 Package 与 File 关系 (新增功能)
	t.Run("Verify Package and File Relations", func(t *testing.T) {
		foundPkgFile := false
		for _, rel := range relations {
			if rel.Type == model.Contain &&
				rel.Source.Kind == model.Package &&
				rel.Source.Name == "com.example.service" &&
				rel.Target.Kind == model.File {
				foundPkgFile = true
				break
			}
		}
		assert.True(t, foundPkgFile, "Should find Package -> CONTAIN -> File relation")
	})

	// 5. 验证局部类 (Local Class) 的层级提取
	t.Run("Verify Local Class Structure", func(t *testing.T) {
		foundLocalClass := false
		for _, rel := range relations {
			// register 方法应该 CONTAIN LocalValidator
			if rel.Type == model.Contain &&
				rel.Source.Name == "register" &&
				rel.Target.Name == "LocalValidator" {
				foundLocalClass = true
				assert.Equal(t, model.Class, rel.Target.Kind)
				// 验证 QN 是否正确包含了方法名作为前缀
				assert.Contains(t, rel.Target.QualifiedName, "CallbackManager.register().LocalValidator")
			}
		}
		assert.True(t, foundLocalClass, "Should extract LocalValidator under register method")
	})

	// 6. 验证 JDK 内置符号解析 (JavaBuiltinTable)
	t.Run("Verify JDK Builtin Resolution", func(t *testing.T) {
		foundSystemOut := false
		foundRunnableType := false

		for _, rel := range relations {
			// 验证 System.out 的解析
			if rel.Type == model.Use && rel.Target.Name == "out" {
				foundSystemOut = true
				assert.Equal(t, "java.lang.System.out", rel.Target.QualifiedName)
				assert.Equal(t, model.Field, rel.Target.Kind)
			}

			// 验证 Runnable 接口类型的解析 (通过 JavaBuiltinTable)
			if rel.Target.Name == "Runnable" {
				foundRunnableType = true
				assert.Equal(t, "java.lang.Runnable", rel.Target.QualifiedName)
				assert.Equal(t, model.Interface, rel.Target.Kind)
			}
		}
		assert.True(t, foundSystemOut, "Should resolve 'out' to java.lang.System.out")
		assert.True(t, foundRunnableType, "Should resolve 'Runnable' to java.lang.Runnable")
	})

	// 7. 验证匿名内部类中的方法调用归属
	t.Run("Verify Chained Call Resolution", func(t *testing.T) {
		foundFullCall := false
		for _, rel := range relations {
			if rel.Type == model.Call && rel.Target.QualifiedName == "java.lang.System.out.println()" {
				foundFullCall = true
				break
			}
		}
		assert.True(t, foundFullCall, "Should resolve full path for chained call: java.lang.System.out.println()")
	})
}

// 验证结构关系：CONTAIN
// 验证结构化依赖 (Extend & Implement)
// 验证方法参数作用域
// 验证 JDK 内置符号表 (JavaBuiltinTable)
// 验证泛型后的类型解析 (Create & Cast)
// 验证异常抛出关系 (Throws)
// 验证库内置类的 IsBuiltin 属性
func TestJavaExtractor_Comprehensive(t *testing.T) {
	// 1. 准备测试文件路径（包含依赖的文件组）
	baseDir := filepath.Join("com", "example")
	modelFile := getTestFilePath(filepath.Join(baseDir, "model", "AbstractBaseEntity.java"))
	coreFile := getTestFilePath(filepath.Join(baseDir, "core", "DataProcessor.java"))
	targetFile := getTestFilePath(filepath.Join(baseDir, "service", "UserServiceImpl.java"))

	// 2. Phase 1: 构建全局上下文 (运行 Collector)
	// 将相关联的文件全部加入 Collection，模拟真实的符号发现环境
	gCtx := runPhase1Collection(t, []string{modelFile, coreFile, targetFile})

	// 3. Phase 2: 运行 Extractor
	ext := java.NewJavaExtractor()
	relations, err := ext.Extract(targetFile, gCtx)
	if err != nil {
		t.Fatalf("Extraction failed for %s: %v", targetFile, err)
	}
	printRelations(relations)

	// 4. 验证结构化依赖 (Extend & Implement)
	t.Run("Verify Structural Hierarchy", func(t *testing.T) {
		foundExtend := false
		foundImplement := false

		for _, rel := range relations {
			// 验证 UserServiceImpl EXTEND AbstractBaseEntity
			if rel.Type == model.Extend &&
				rel.Source.Name == "UserServiceImpl" &&
				rel.Target.QualifiedName == "com.example.model.AbstractBaseEntity" {
				foundExtend = true
			}
			// 验证 UserServiceImpl IMPLEMENT DataProcessor
			if rel.Type == model.Implement &&
				rel.Source.Name == "UserServiceImpl" &&
				rel.Target.QualifiedName == "com.example.core.DataProcessor" {
				foundImplement = true
			}
		}
		assert.True(t, foundExtend, "Should resolve inheritance to com.example.model.AbstractBaseEntity")
		assert.True(t, foundImplement, "Should resolve implementation to com.example.core.DataProcessor")
	})

	// 5. 验证方法参数作用域 (上户口后的引用识别)
	t.Run("Verify Parameter Scope Call", func(t *testing.T) {
		foundParamCall := false
		for _, rel := range relations {
			// 关键：现在 Name 应该是 toUpperCase 了
			if rel.Type == model.Call && rel.Target.Name == "toUpperCase" {
				// 且 QN 包含了参数的路径
				if strings.Contains(rel.Target.QualifiedName, "batchId.toUpperCase") {
					foundParamCall = true
				}
			}
		}
		assert.True(t, foundParamCall)
	})

	// 6. 验证 JDK 内置符号表 (JavaBuiltinTable)
	t.Run("Verify JDK Builtin Resolution", func(t *testing.T) {
		foundUUIDCall := false
		for _, rel := range relations {
			// 检查是否调用了 UUID 里的方法
			if rel.Type == model.Call && strings.HasPrefix(rel.Target.QualifiedName, "java.util.UUID") {
				foundUUIDCall = true
			}
		}
		assert.True(t, foundUUIDCall, "Should resolve call to java.util.UUID methods")
	})

	// 7. 验证泛型后的类型解析 (Create & Cast)
	t.Run("Verify Create and Cast", func(t *testing.T) {
		foundArrayListCreate := false
		foundStringCast := false

		for _, rel := range relations {
			// 验证 new ArrayList<>()
			if rel.Type == model.Create && rel.Target.Name == "ArrayList" {
				foundArrayListCreate = true
				assert.Equal(t, "java.util.ArrayList", rel.Target.QualifiedName)
			}
			// 验证 (String) rawData
			if rel.Type == model.Cast && rel.Target.Name == "String" {
				foundStringCast = true
				assert.Equal(t, "java.lang.String", rel.Target.QualifiedName)
			}
		}
		assert.True(t, foundArrayListCreate, "Should extract object creation for ArrayList")
		assert.True(t, foundStringCast, "Should extract type cast to String")
	})

	// 8. 验证异常抛出关系 (Throws)
	t.Run("Verify Exception Throws", func(t *testing.T) {
		foundException := false
		for _, rel := range relations {
			if rel.Type == model.Throw && rel.Target.Name == "RuntimeException" {
				foundException = true
				assert.Equal(t, "java.lang.RuntimeException", rel.Target.QualifiedName)
			}
		}
		assert.True(t, foundException, "Should resolve Exception in throws clause")
	})

	// 9. 验证库内置类的 IsBuiltin 属性
	t.Run("Verify IsBuiltin Attribute for JDK Classes", func(t *testing.T) {
		// 在 UserServiceImpl.java 中，String 应该被识别为内置类
		foundStringRel := false
		for _, rel := range relations {
			if rel.Target.Name == "String" {
				foundStringRel = true
				assert.NotNil(t, rel.Target.Extra, "Extra should not be nil for Builtin class")
				assert.NotNil(t, rel.Target.Extra.ClassExtra, "ClassExtra should be populated")
				assert.True(t, rel.Target.Extra.ClassExtra.IsBuiltin, "String must be marked as IsBuiltin")
			}
		}
		assert.True(t, foundStringRel, "Should find relationship involving String")
	})

	// 10. 验证访问父类的Field
	t.Run("Verify SuperClass Field Access", func(t *testing.T) {
		foundIdField := false
		for _, rel := range relations {
			if rel.Type == model.Use && rel.Target.Name == "id" {
				foundIdField = true
				// 验证 QN 是否正确指向了父类
				assert.Equal(t, "com.example.model.AbstractBaseEntity.id", rel.Target.QualifiedName)
			}
		}
		assert.True(t, foundIdField, "Should resolve 'id' to the super class field definition")
	})
}

// 验证元注解的解析 (Annotation -> Annotation)
// 验证注解属性成员的类型引用
// 验证通配符 Import 对解析的影响
// 验证注解参数中的静态成员引用 (Enum Constants)
func TestJavaExtractor_AnnotationMetadata(t *testing.T) {
	// 1. 准备路径
	targetFile := getTestFilePath(filepath.Join("com", "example", "annotation", "Loggable.java"))

	// 2. Phase 1: 构建上下文
	gCtx := runPhase1Collection(t, []string{targetFile})

	// 3. Phase 2: 提取关系
	ext := java.NewJavaExtractor()
	relations, err := ext.Extract(targetFile, gCtx)
	if err != nil {
		t.Fatalf("Extraction failed: %v", err)
	}
	printRelations(relations)

	// 4. 验证元注解的解析 (Annotation -> Annotation)
	t.Run("Verify Meta-Annotations Resolution", func(t *testing.T) {
		foundRetention := false
		foundTarget := false

		for _, rel := range relations {
			if rel.Type == model.Annotation && rel.Source.Name == "Loggable" {
				// 验证 @Retention(RetentionPolicy.RUNTIME) 是否解析到了 java.lang.annotation.Retention
				if rel.Target.QualifiedName == "java.lang.annotation.Retention" {
					foundRetention = true
					assert.Equal(t, model.KAnnotation, rel.Target.Kind)
				}
				// 验证 @Target(...) 是否解析到了 java.lang.annotation.Target
				if rel.Target.QualifiedName == "java.lang.annotation.Target" {
					foundTarget = true
				}
			}
		}
		assert.True(t, foundRetention, "Should resolve @Retention to java.lang.annotation.Retention")
		assert.True(t, foundTarget, "Should resolve @Target to java.lang.annotation.Target")
	})

	// 5. 验证注解属性成员的类型引用
	t.Run("Verify Annotation Member Types", func(t *testing.T) {
		foundLevelType := false
		for _, rel := range relations {
			// String level() -> 应该产生一个 RETURN 关系指向 String
			if rel.Type == model.Return &&
				rel.Source.QualifiedName == "com.example.annotation.Loggable.level()" &&
				rel.Target.QualifiedName == "java.lang.String" {
				foundLevelType = true
				assert.True(t, rel.Target.Extra.ClassExtra.IsBuiltin, "String should be marked as Builtin")
			}
		}
		assert.True(t, foundLevelType, "Annotation member 'level' should have RETURN relation to String")
	})

	// 6. 验证通配符 Import 对解析的影响
	t.Run("Verify Wildcard Import Impact", func(t *testing.T) {
		// 在 Loggable.java 中有 import java.lang.annotation.*;
		// 检查 Extractor 是否正确生成了对该包的 Import 关系
		foundWildcardImport := false
		for _, rel := range relations {
			if rel.Type == model.Import &&
				rel.Target.QualifiedName == "java.lang.annotation" {
				foundWildcardImport = true
				break
			}
		}
		assert.True(t, foundWildcardImport, "Should extract import relation for java.lang.annotation")
	})

	// 7. 验证注解参数中的静态成员引用 (Enum Constants)
	t.Run("Verify Static Member Resolution in Annotations", func(t *testing.T) {
		foundRuntime := false
		foundType := false
		foundMethod := false

		for _, rel := range relations {
			if rel.Type == model.Use {
				// 验证 RetentionPolicy.RUNTIME
				if rel.Target.Name == "RUNTIME" {
					foundRuntime = true
					assert.Equal(t, "java.lang.annotation.RetentionPolicy.RUNTIME", rel.Target.QualifiedName)
					assert.Equal(t, model.Field, rel.Target.Kind)
				}
				// 验证 ElementType.TYPE
				if rel.Target.Name == "TYPE" {
					foundType = true
					assert.Equal(t, "java.lang.annotation.ElementType.TYPE", rel.Target.QualifiedName)
				}
				// 验证 ElementType.METHOD
				if rel.Target.Name == "METHOD" {
					foundMethod = true
					assert.Equal(t, "java.lang.annotation.ElementType.METHOD", rel.Target.QualifiedName)
				}
			}
		}

		assert.True(t, foundRuntime, "Should resolve RetentionPolicy.RUNTIME to its full builtin QN")
		assert.True(t, foundType, "Should resolve ElementType.TYPE to its full builtin QN")
		assert.True(t, foundMethod, "Should resolve ElementType.METHOD to its full builtin QN")
	})
}

// 验证变长参数与数组类型引用
// 验证复杂注解解析 (带属性值)
func TestJavaExtractor_ConfigService(t *testing.T) {
	// 1. 准备测试文件路径
	targetFile := getTestFilePath(filepath.Join("com", "example", "config", "ConfigService.java"))

	// 2. Phase 1: 收集定义
	gCtx := runPhase1Collection(t, []string{targetFile})

	// 3. Phase 2: 提取关系
	ext := java.NewJavaExtractor()
	relations, err := ext.Extract(targetFile, gCtx)
	assert.NoError(t, err)
	printRelations(relations)

	// 4. 验证变长参数与数组类型引用
	t.Run("Verify Varargs and Array Types", func(t *testing.T) {
		foundStringArray := false
		foundObjectVarargs := false

		for _, rel := range relations {
			if rel.Type == model.Parameter && rel.Source.Name == "updateConfigs" {
				// 验证 String[] -> java.lang.String (需剥离数组符号)
				if rel.Target.QualifiedName == "java.lang.String" {
					foundStringArray = true
				}
				// 验证 Object... -> java.lang.Object (需剥离变长符号)
				if rel.Target.QualifiedName == "java.lang.Object" {
					foundObjectVarargs = true
					assert.True(t, rel.Target.Extra.ClassExtra.IsBuiltin)
				}
			}
		}
		assert.True(t, foundStringArray, "Should resolve String[] parameter type to java.lang.String")
		assert.True(t, foundObjectVarargs, "Should resolve Object... parameter type to java.lang.Object")
	})

	// 5. 验证复杂注解解析 (带属性值)
	t.Run("Verify Complex Annotations", func(t *testing.T) {
		foundSuppressWarnings := false
		foundDeprecated := false

		for _, rel := range relations {
			if rel.Type == model.Annotation && rel.Source.Name == "legacyMethod" {
				// 验证 @SuppressWarnings({"unchecked", "rawtypes"}) -> java.lang.SuppressWarnings
				if rel.Target.QualifiedName == "java.lang.SuppressWarnings" {
					foundSuppressWarnings = true
				}
				// 验证 @Deprecated(since = "1.2", forRemoval = true) -> java.lang.Deprecated
				if rel.Target.QualifiedName == "java.lang.Deprecated" {
					foundDeprecated = true
				}
			}
		}
		assert.True(t, foundSuppressWarnings, "Should resolve SuppressWarnings even with array attributes")
		assert.True(t, foundDeprecated, "Should resolve Deprecated even with named attributes")
	})
}

// 验证接口多继承 (EXTEND)
// 验证异常抛出关系 (THROW)
// 验证默认方法内的调用 (Default Method)
func TestJavaExtractor_InterfaceAndGenerics(t *testing.T) {
	// 1. 准备测试文件路径
	targetFile := getTestFilePath(filepath.Join("com", "example", "core", "DataProcessor.java"))

	// 2. 执行收集与提取
	gCtx := runPhase1Collection(t, []string{targetFile})
	ext := java.NewJavaExtractor()
	relations, err := ext.Extract(targetFile, gCtx)
	assert.NoError(t, err)
	printRelations(relations)

	// 3. 验证接口多继承 (EXTEND)
	t.Run("Verify Interface Multi-Inheritance", func(t *testing.T) {
		foundRunnable := false
		foundAutoCloseable := false
		for _, rel := range relations {
			if rel.Type == model.Extend && rel.Source.Name == "DataProcessor" {
				if rel.Target.QualifiedName == "java.lang.Runnable" {
					foundRunnable = true
				}
				if rel.Target.QualifiedName == "java.lang.AutoCloseable" {
					foundAutoCloseable = true
				}
			}
		}
		assert.True(t, foundRunnable)
		assert.True(t, foundAutoCloseable)
	})

	// 4. 验证异常抛出关系 (THROW)
	t.Run("Verify Method Throws", func(t *testing.T) {
		foundRuntimeException := false
		foundException := false

		for _, rel := range relations {
			if rel.Type == model.Throw && rel.Source.Name == "processAll" {
				if rel.Target.QualifiedName == "java.lang.RuntimeException" {
					foundRuntimeException = true
				}
				if rel.Target.QualifiedName == "java.lang.Exception" {
					foundException = true
				}
			}
		}
		assert.True(t, foundRuntimeException, "Method processAll should throw RuntimeException")
		assert.True(t, foundException, "Method processAll should throw Exception")
	})

	// 5. 验证默认方法内的调用 (Default Method)
	t.Run("Verify Default Method Logic", func(t *testing.T) {
		foundSystemOut := false
		for _, rel := range relations {
			// stop() 默认方法中调用了 System.out.println
			if rel.Type == model.Use && rel.Source.Name == "stop" {
				// 这里的路径取决于你 resolveWithPrefix 的处理，通常指向 System.out
				if rel.Target.QualifiedName == "java.lang.System.out" {
					foundSystemOut = true
				}
			}
		}
		assert.True(t, foundSystemOut, "Should resolve System.out usage inside default method")
	})
}

// 验证结构化定义关系（Implement、Return、Parameter、Contain）
// 验证方法体内的动作关系（Use）
// 验证内部类 QN 拼接逻辑
func TestJavaExtractor_BaseEntity(t *testing.T) {
	targetFile := getTestFilePath(filepath.Join("com", "example", "model", "AbstractBaseEntity.java"))

	gCtx := runPhase1Collection(t, []string{targetFile})
	ext := java.NewJavaExtractor()
	relations, err := ext.Extract(targetFile, gCtx)
	assert.NoError(t, err)
	printRelations(relations)

	const entityQN = "com.example.model.AbstractBaseEntity"
	const metaQN = "com.example.model.AbstractBaseEntity.EntityMeta"

	// 1. 验证结构化定义关系 (Structural Relations)
	t.Run("Verify Definitions", func(t *testing.T) {
		foundSerializable := false
		foundGetIdReturn := false
		foundSetIdParam := false
		foundInnerClass := false

		for _, rel := range relations {
			// 接口实现
			if rel.Type == model.Implement && rel.Source.QualifiedName == entityQN {
				if rel.Target.QualifiedName == "java.io.Serializable" {
					foundSerializable = true
				}
			}
			// 方法返回类型 (ID)
			if rel.Type == model.Return && rel.Source.QualifiedName == entityQN+".getId()" {
				if rel.Target.Name == "ID" {
					foundGetIdReturn = true
				}
			}
			// 方法参数类型 (ID)
			if rel.Type == model.Parameter && rel.Source.QualifiedName == entityQN+".setId(ID)" {
				if rel.Target.Name == "ID" {
					foundSetIdParam = true
				}
			}
			// 内部类包含
			if rel.Type == model.Contain && rel.Source.QualifiedName == entityQN {
				if rel.Target.QualifiedName == metaQN {
					foundInnerClass = true
				}
			}
		}
		assert.True(t, foundSerializable, "Should implement Serializable")
		assert.True(t, foundGetIdReturn, "getId() should return ID")
		assert.True(t, foundSetIdParam, "setId(ID) should have ID parameter")
		assert.True(t, foundInnerClass, "Should contain EntityMeta")
	})

	// 2. 验证方法体内的动作关系 (Action/Behavioral Relations)
	t.Run("Verify Method Body Actions", func(t *testing.T) {
		foundFieldUse := false

		for _, rel := range relations {
			// 核心修复点：验证 setId 方法对 id 字段的访问 (this.id = id)
			// Source 是方法，Target 是字段
			if rel.Type == model.Use && rel.Source.QualifiedName == entityQN+".setId(ID)" {
				if rel.Target.QualifiedName == entityQN+".id" {
					foundFieldUse = true
				}
			}
		}
		assert.True(t, foundFieldUse, "Method setId(ID) should USE field id")
	})

	// 3. 验证内部类 QN 拼接逻辑
	t.Run("Verify Inner Class Methods", func(t *testing.T) {
		foundStringReturn := false
		for _, rel := range relations {
			if rel.Type == model.Return && rel.Source.QualifiedName == metaQN+".getTableName()" {
				if rel.Target.QualifiedName == "java.lang.String" {
					foundStringReturn = true
				}
			}
		}
		assert.True(t, foundStringReturn, "EntityMeta.getTableName() should return String")
	})
}

// 验证枚举结构 (CONTAIN)
// 验证构造函数中的字段使用 (USE)
// 验证方法返回类型 (RETURN)
func TestJavaExtractor_EnumErrorCode(t *testing.T) {
	// 1. 准备测试文件路径
	targetFile := getTestFilePath(filepath.Join("com", "example", "model", "ErrorCode.java"))

	// 2. 执行收集与提取
	gCtx := runPhase1Collection(t, []string{targetFile})
	ext := java.NewJavaExtractor()
	relations, err := ext.Extract(targetFile, gCtx)
	assert.NoError(t, err)
	printRelations(relations)

	const enumQN = "com.example.model.ErrorCode"

	// 3. 验证枚举结构 (CONTAIN)
	t.Run("Verify Enum Structure", func(t *testing.T) {
		foundConstants := map[string]bool{"USER_NOT_FOUND": false, "NAME_EMPTY": false}
		foundFields := map[string]bool{"code": false, "message": false}
		foundMethods := map[string]bool{"getCode": false, "getMessage": false, "ErrorCode": false}

		for _, rel := range relations {
			if rel.Type == model.Contain && rel.Source.QualifiedName == enumQN {
				name := rel.Target.Name
				if _, ok := foundConstants[name]; ok {
					foundConstants[name] = true
				}
				if _, ok := foundFields[name]; ok {
					foundFields[name] = true
				}
				if _, ok := foundMethods[name]; ok {
					foundMethods[name] = true
				}
			}
		}

		for name, found := range foundConstants {
			assert.True(t, found, "Enum should contain constant: "+name)
		}
		assert.True(t, foundFields["code"], "Enum should contain field: code")
		assert.True(t, foundMethods["getCode"], "Enum should contain method: getCode")
		assert.True(t, foundMethods["ErrorCode"], "Enum should contain constructor: ErrorCode")
	})

	// 4. 验证构造函数中的字段使用 (USE)
	t.Run("Verify Constructor Field Usage", func(t *testing.T) {
		foundCodeUse := false
		foundMessageUse := false

		// 构造函数的 QN 通常为 enumQN.ErrorCode
		const constructorQN = enumQN + ".ErrorCode(int,String)"

		for _, rel := range relations {
			if rel.Type == model.Use && rel.Source.QualifiedName == constructorQN {
				if rel.Target.QualifiedName == enumQN+".code" {
					foundCodeUse = true
				}
				if rel.Target.QualifiedName == enumQN+".message" {
					foundMessageUse = true
				}
			}
		}
		assert.True(t, foundCodeUse, "Constructor should USE field 'code'")
		assert.True(t, foundMessageUse, "Constructor should USE field 'message'")
	})

	// 5. 验证方法返回类型 (RETURN)
	t.Run("Verify Method Returns", func(t *testing.T) {
		foundStringReturn := false
		for _, rel := range relations {
			if rel.Type == model.Return && rel.Source.Name == "getMessage" {
				// String 应通过 JavaBuiltinTable 解析为 java.lang.String
				if rel.Target.QualifiedName == "java.lang.String" {
					foundStringReturn = true
				}
			}
		}
		assert.True(t, foundStringReturn, "getMessage should return java.lang.String")
	})
}

// 验证继承关系 (EXTEND)
// 验证构造函数参数与 Super 调用 (PARAMETER & CALL)
// 验证跨类调用 (CALL to getMessage)
// 验证静态常量字段 (CONTAIN)
func TestJavaExtractor_NotificationException(t *testing.T) {
	// 1. 准备测试文件路径
	targetFile := getTestFilePath(filepath.Join("com", "example", "model", "NotificationException.java"))
	supplementaryFile := getTestFilePath(filepath.Join("com", "example", "model", "ErrorCode.java"))

	// 2. 执行收集与提取
	gCtx := runPhase1Collection(t, []string{targetFile, supplementaryFile})
	ext := java.NewJavaExtractor()
	relations, err := ext.Extract(targetFile, gCtx)
	assert.NoError(t, err)
	printRelations(relations)

	const exceptionQN = "com.example.model.NotificationException"

	// 3. 验证继承关系 (EXTEND)
	t.Run("Verify Inheritance", func(t *testing.T) {
		foundExtendException := false
		for _, rel := range relations {
			if rel.Type == model.Extend && rel.Source.QualifiedName == exceptionQN {
				// Exception 应该通过 JavaBuiltinTable 解析为 java.lang.Exception
				if rel.Target.QualifiedName == "java.lang.Exception" {
					foundExtendException = true
				}
			}
		}
		assert.True(t, foundExtendException, "NotificationException should extend java.lang.Exception")
	})

	// 4. 验证构造函数参数与 Super 调用 (PARAMETER & CALL)
	t.Run("Verify Constructor and Super Call", func(t *testing.T) {
		foundThrowableParam := false
		foundSuperCall := false

		const constructor1QN = exceptionQN + ".NotificationException(String,Throwable)"

		for _, rel := range relations {
			// 验证参数类型 Throwable (内置类)
			if rel.Type == model.Parameter && rel.Source.QualifiedName == constructor1QN {
				if rel.Target.QualifiedName == "java.lang.Throwable" {
					foundThrowableParam = true
				}
			}
			// 验证 super(message, cause) 产生的 Call 关系
			// 注意：在 Java 中 super() 指向父类构造函数，QN 通常被解析为 java.lang.Exception.Exception()
			if rel.Type == model.Call && rel.Source.QualifiedName == constructor1QN {
				if strings.Contains(rel.Target.QualifiedName, "Exception.Exception()") || rel.Target.Name == "super" {
					foundSuperCall = true
				}
			}
		}
		assert.True(t, foundThrowableParam, "Constructor should have java.lang.Throwable parameter")
		assert.True(t, foundSuperCall, "Constructor should have Exception.Exception call")
		// 如果你的 Extractor 暂不支持 super 关键字的精确解析，这里可以根据 printRelations 的实际输出微调
	})

	// 5. 验证跨类调用 (CALL to getMessage)
	t.Run("Verify Cross-Class Method Call", func(t *testing.T) {
		foundGetMessageCall := false

		for _, rel := range relations {
			// 验证在 NotificationException(ErrorCode code) 中调用了 code.getMessage()
			if rel.Type == model.Call && strings.Contains(rel.Source.QualifiedName, "NotificationException(ErrorCode)") {
				if rel.Target.QualifiedName == exceptionQN+".NotificationException(ErrorCode).code.getMessage()" {
					foundGetMessageCall = true
				}
			}
		}
		assert.True(t, foundGetMessageCall, "Should detect call to code.getMessage()")
	})

	// 6. 验证静态常量字段 (CONTAIN)
	t.Run("Verify Static Field", func(t *testing.T) {
		foundSerialField := false
		for _, rel := range relations {
			if rel.Type == model.Contain && rel.Source.QualifiedName == exceptionQN {
				if rel.Target.Name == "serialVersionUID" {
					foundSerialField = true
				}
			}
		}
		assert.True(t, foundSerialField, "Should contain serialVersionUID field")
	})
}

// 验证字段与静态方法调用 (UUID.randomUUID())
// 验证内部类 (Inner Class CONTAIN)
// 验证静态导入解析 (Static Imports)
// 验证构造函数中的 Field Access (this)
func TestJavaExtractor_User(t *testing.T) {
	// 1. 准备测试文件路径
	targetFile := getTestFilePath(filepath.Join("com", "example", "model", "User.java"))

	// 2. 执行收集与提取
	gCtx := runPhase1Collection(t, []string{targetFile})
	ext := java.NewJavaExtractor()
	relations, err := ext.Extract(targetFile, gCtx)
	assert.NoError(t, err)
	printRelations(relations)

	const userQN = "com.example.model.User"
	const addonInfoQN = "com.example.model.User.AddonInfo"

	// 3. 验证字段与静态方法调用 (UUID.randomUUID)
	t.Run("Verify Static Method and Field", func(t *testing.T) {
		foundUUIDCall := false
		foundToStringCall := false

		for _, rel := range relations {
			// 验证 DEFAULT_ID 初始化时的 UUID.randomUUID()
			if rel.Type == model.Call && strings.Contains(rel.Source.QualifiedName, "DEFAULT_ID") {
				if rel.Target.QualifiedName == "java.util.UUID.randomUUID()" {
					foundUUIDCall = true
				}
				// 验证 .toString() 调用
				if rel.Target.Name == "toString" {
					foundToStringCall = true
				}
			}
		}
		assert.True(t, foundUUIDCall, "Should detect static call to java.util.UUID.randomUUID()")
		assert.True(t, foundToStringCall, "Should detect toString() call on UUID object")
	})

	// 4. 验证内部类 (Inner Class CONTAIN)
	t.Run("Verify Inner Class Structure", func(t *testing.T) {
		foundAddonInfo := false
		foundInnerField := false

		for _, rel := range relations {
			// User CONTAIN AddonInfo
			if rel.Type == model.Contain && rel.Source.QualifiedName == userQN {
				if rel.Target.QualifiedName == addonInfoQN {
					foundAddonInfo = true
				}
			}
			// AddonInfo CONTAIN birthday
			if rel.Type == model.Contain && rel.Source.QualifiedName == addonInfoQN {
				if rel.Target.Name == "birthday" {
					foundInnerField = true
				}
			}
		}
		assert.True(t, foundAddonInfo, "User should contain inner class AddonInfo")
		assert.True(t, foundInnerField, "AddonInfo should contain field birthday")
	})

	// 5. 验证静态导入解析 (Static Imports)
	t.Run("Verify Static Import Usage", func(t *testing.T) {
		foundDaysUse := false
		foundTimeUnitCall := false

		// chooseUnit 方法内部逻辑
		for _, rel := range relations {
			if strings.Contains(rel.Source.QualifiedName, "chooseUnit(long)") {
				// 验证对静态导入 DAYS 的使用
				// 根据 java_extractor 的 resolveTargetElement，静态导入通常解析到其所属类
				if strings.Contains(rel.Target.QualifiedName, "java.util.concurrent.TimeUnit.DAYS") {
					foundDaysUse = true
				}
				// 验证 DAYS.convert(...) 调用
				if rel.Type == model.Call && rel.Target.QualifiedName == "java.util.concurrent.TimeUnit.DAYS.convert()" {
					foundTimeUnitCall = true
				}
			}
		}
		assert.True(t, foundDaysUse, "Should resolve static import DAYS to TimeUnit.DAYS")
		assert.True(t, foundTimeUnitCall, "Should detect call to convert method")
	})

	// 6. 验证构造函数中的 Field Access (this)
	t.Run("Verify This Reference", func(t *testing.T) {
		foundIdAssignment := false
		for _, rel := range relations {
			if rel.Type == model.Use && rel.Source.Name == "User" && rel.Source.Kind == model.Method {
				if rel.Target.QualifiedName == userQN+".id" {
					foundIdAssignment = true
				}
			}
		}
		assert.True(t, foundIdAssignment, "Constructor should assign value to this.id")
	})
}
