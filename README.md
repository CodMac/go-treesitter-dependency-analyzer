# 🌳 Go Tree-sitter Dependency Analyzer

**Go Tree-sitter Dependency Analyzer** 是一个工业级的高性能源码依赖分析引擎。它利用 **Tree-sitter** 的精确解析能力，通过 **两阶段分析（Two-Phase Analysis）** 策略，能够从大规模代码库中提取出具有语义深度的代码元素（类、方法、参数）及其相互依赖关系。

该工具不仅仅是简单的文本扫描，它具备**作用域感知**、**继承链追踪**以及**内置符号推导**能力，能够还原出接近编译器级别的依赖图谱。

## ✨ 核心特性

* **⚡️ 高性能并发架构**：基于 Go 协程的 Worker Pool 设计，支持万级文件并发解析，秒级完成中大型项目全扫描。
* **🧩 两阶段语义分析**：
* **Phase 1 (Collection)**：构建全局符号表，自动处理 Package、Imports 及符号的全限定名（Qualified Name）。
* **Phase 2 (Extraction)**：执行深度依赖提取，支持跨文件的符号消歧。


* **☕️ 现代 Java 特性深度支持**：
* **Java 17 Record**：完美识别 Record 的隐式组件、字段及访问器方法（Accessor）。
* **作用域追踪**：精确到**方法参数级**的 Qualified Name（如 `pkg.Class.method.param`），彻底解决同名变量混淆。
* **内置库感知**：内置 100+ 核心 JDK 符号表（`java.lang`, `java.util`, `java.time` 等），支持隐式类型解析。
* **继承链解析**：支持沿 `extends` 链向上搜索字段与方法调用。


* **🔗 精细化依赖模型**：
* **结构关系**：`EXTEND`, `IMPLEMENT`, `CONTAIN`, `ANNOTATION`。
* **行为关系**：`CALL` (方法调用), `CREATE` (实例化), `USE` (字段访问), `CAST` (强转), `THROW` (异常抛出)。
* **方法特征**：`RETURN` (返回类型), `PARAMETER` (参数类型引用)。



## ⚙️ 核心解析逻辑

工具通过 `ElementExtra` 结构捕获了极丰富的代码元数据：

* **修饰符与注解**：提取 `public`, `static`, `final` 及 `@Service`, `@Override` 等注解。
* **方法特征**：记录是否为构造函数、参数列表原始签名、抛出的异常类型等。
* **类特征**：记录父类、接口列表、是否为抽象类或内置类。

## 📄 输出 JSON 示例 (Rich Metadata)

每一行输出都包含完整的上下文，非常适合作为 AI 模型、代码审计工具或架构演化分析的输入。

```json
{
  "Type": "CALL",
  "Source": {
    "Kind": "METHOD",
    "Name": "processOrder",
    "QualifiedName": "com.shop.OrderService.processOrder",
    "Path": "src/main/java/com/shop/OrderService.java",
    "Extra": {
      "MethodExtra": {
        "ReturnType": "boolean",
        "Parameters": ["Order order", "User user"]
      }
    }
  },
  "Target": {
    "Kind": "METHOD",
    "Name": "price",
    "QualifiedName": "com.shop.model.Order.price",
    "Extra": {
      "MethodExtra": { "IsConstructor": false }
    }
  },
  "Location": {
    "FilePath": "src/main/java/com/shop/OrderService.java",
    "StartLine": 42,
    "StartColumn": 25
  }
}

```

## 🚀 快速开始

### 1. 环境准备

需要安装 **Go 1.25+** 和 **C 编译器**（用于编译 Tree-sitter C 库）。

### 2. 构建与运行

```bash
# 构建
CGO_ENABLED=1 go build -o analyzer main.go

# 运行分析 (以 Java 项目为例)
./analyzer -lang java -path ./your-project-path -filter ".*\.java$" > dependencies.jsonl

```

### 3. 命令行参数

| 参数 | 说明 |
| --- | --- |
| `-lang` | 目标语言（目前 `java` 支持最完整，`go` 开发中） |
| `-path` | 待分析的项目根目录 |
| `-filter` | 正则表达式，过滤特定文件后缀 |
| `-jobs` | 并发线程数（默认 4） |
| `-output` | 指定输出文件路径 |

## 🧪 自动化测试

项目拥有极高的测试覆盖率，特别是针对 Java 的各种边缘情况：

* `TestJavaCollector_ModernRecord`: 验证 Java 17 Record 解析。
* `TestJavaExtractor_Comprehensive`: 验证跨包继承与泛型擦除后的符号解析。
* `TestJavaExtractor_AnnotationMetadata`: 验证注解内静态枚举常量的引用。

```bash
CGO_ENABLED=1 go test ./x/java/... -v

```

## 🛠️ 扩展指南

如果你想支持新语言（如 Rust, Python）：

1. 在 `model/element.go` 中定义该语言特有的 `ElementKind`。
2. 在 `x/` 下实现 `Collector`（扫描定义）和 `Extractor`（扫描引用）。
3. 利用 `GlobalContext.ResolveSymbol` 挂载你语言特有的解析逻辑。
