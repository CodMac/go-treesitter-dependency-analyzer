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
		rootNode, sourceBytes, err := javaParser.ParseFile(file, false, false)
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
				assert.Contains(t, rel.Target.QualifiedName, "CallbackManager.register.LocalValidator")
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
			if rel.Type == model.Call && rel.Target.QualifiedName == "java.lang.System.out.println" {
				foundFullCall = true
				break
			}
		}
		assert.True(t, foundFullCall, "Should resolve full path for chained call: java.lang.System.out.println")
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
				rel.Source.QualifiedName == "com.example.annotation.Loggable.level" &&
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
	// 1. 准备测试文件路径 (假设 AbstractBaseEntity 也在扫描路径中)
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
