# Go-TreeSitter-Dependency-Analyzer

一个高性能、多语言支持的 Go 语言命令行工具，用于基于 Tree-sitter 分析软件源码间的依赖调用关系。

## ✨ 核心特性 (Features)

  * **多语言支持:** 通过 Tree-sitter 动态加载不同语言的语法解析器（如 Go, Python, Java, JavaScript 等）。
  * **高精度 AST 解析:** 利用 Tree-sitter 生成精确的抽象语法树 (AST)。
  * **丰富的依赖模型:** 提取函数/方法级别的调用关系、继承、实现和使用关系等，共支持 14 种依赖类型。
  * **JSONL 输出:** 将分析结果以 JSON Lines 格式输出，方便集成到其他数据处理管道或图形数据库中。
  * **高并发分析:** 利用 Go 协程实现多文件并行解析，提高分析效率。

## ⚙️ 架构设计 (Architecture)

为了保证多语言的可扩展性和代码的复用性，项目采用清晰的三层架构设计：

| 层次 | 职责 | 核心组件 | 技术点 |
| :--- | :--- | :--- | :--- |
| **0. 底层解析层** | 源码读取，AST 生成 | `parser/` | Tree-sitter Go Bindings |
| **1. 语言适配层** | AST 遍历，提取语言特征并转换为通用模型 | `extractor/` | 特定语言的 Tree-sitter Query |
| **2. 模型与输出层** | 收集关系，构建模型，格式化输出 | `model/`, `output/` | Go Struct, JSONL |

## 📐 核心数据模型 (Core Data Model)

分析结果的核心数据结构是 `DependencyRelation`，它描述了**源 (Source)** 与**目标 (Target)** 之间发生的特定**关系 (Type)**。

### 1\. 依赖关系结构 (`DependencyRelation`)

| 字段名称 | 类型 | 描述 |
| :--- | :--- | :--- |
| **`Type`** | `string` | 依赖关系的类型（见下方表格） |
| **`Source`** | `*CodeElement` | 关系的发起方（调用者、导入者等） |
| **`Target`** | `*CodeElement` | 关系的指向方（被调用函数、被导入包等） |
| **`Location`** | `*Location` | 关系发生的源码位置 (文件、行号) |

### 2\. 代码元素结构 (`CodeElement`)

代表源码中的一个可识别实体（如文件、函数、变量、类型等）。

| 字段名称 | 类型 | 描述 |
| :--- | :--- | :--- |
| **`Kind`** | `string` | 元素类型 (e.g., `FUNCTION`, `CLASS`, `PACKAGE`, `FILE`) |
| **`Name`** | `string` | 元素的名称 |
| **`QualifiedName`**| `string` | 元素的完整限定名称 (e.g., `pkg.Struct.Method`) |
| **`Path`** | `string` | 元素所在的文件路径 |

## 🔗 支持的依赖类型 (Supported Dependency Types)

工具支持提取以下 14 种依赖关系，借鉴了 `multilang-depends/depends` 项目的依赖类型定义：

| Type | 描述 | 示例场景 |
| :--- | :--- | :--- |
| **`CALL`** | 一个函数/方法调用了另一个函数/方法。 | `foo()` 调用了 `bar()`。 |
| **`IMPORT`** | 文件或模块导入/包含了另一个模块或头文件。 | Go 的 `import "fmt"`, C 的 `#include`。 |
| **`CONTAIN`** | 一个代码块包含另一个代码实体（如类包含成员变量，函数包含局部变量）。 | `class A { B b; }` |
| **`PARAMETER`** | 函数/方法定义中使用了某个类型作为参数。 | `void foo(TypeX x)`。 |
| **`RETURN`** | 函数/方法返回了某个类型。 | `TypeY bar() { return y; }`。 |
| **`THROW`** | 函数/方法抛出了某个异常类型。 | `throw new Exception()`。 |
| **`IMPLEMENT`** | 类实现了接口，或函数实现了原型/声明。 | `class A implements I`。 |
| **`EXTENDS`** | 类继承了另一个父类。 | `class Child extends Parent`。 |
| **`CREATE`** | 函数/方法内部创建了某个类型的实例。 | `obj = new MyClass()`。 |
| **`USE`** | 表达式或代码块使用了变量、常量或类型（通用引用）。 | 使用全局变量或常量。 |
| **`CAST`** | 表达式被强制转换为某个类型。 | `(int)d` 或 `static_cast<int>(d)`。 |
| **`ANNOTATION`**| 代码实体被注解或装饰器修饰。 | Java 的 `@Override`, Python 的 `@decorator`。 |
| **`MIXIN`** | 类包含了模块，复用其方法和特性。 | Ruby 的 `include Logger`。 |
| **`IMPL_LINK`** | 隐式实现链接，用于处理 C/C++ 等语言中调用和实现分离的场景。 | 调用原型与实际多重实现体之间的链接。 |

## 💡 使用方法 (Usage)

*(此部分将在实现后完善，预计包含以下内容)*

```bash
# 构建项目
go build -o dep_analyzer main.go

# 运行分析 (分析 Go 语言项目)
./dep_analyzer analyze --lang go --path /path/to/your/project > results.jsonl

# 运行分析 (分析 Python 语言项目)
./dep_analyzer analyze --lang python --path /path/to/your/python/project > results.jsonl
```

## 贡献 (Contributing)

欢迎提交 Issue 或 Pull Request 来改进此工具，特别是增加对新语言的适配。