# Go Tree-sitter Dependency Analyzer

一个基于 **Tree-sitter** 的高性能、多语言（当前深耕 Java）源代码依赖分析引擎。它能够深入理解代码语义，跨越文件边界，提取出包含继承、调用、引用及现代语言特性的完整依赖图谱。

## ✨ 核心特性

* **深度语义提取**：不仅仅是正则匹配，而是基于 AST（抽象语法树）解析出包含类、方法、字段、注解等层级的全量定义。
* **跨文件符号解析**：具备全局上下文感知能力，能够准确处理跨类的继承链、静态导入及接口实现。
* **现代 Java 全特性支持**：
* **Java 17+**：支持 `Record`（紧凑构造函数、隐式访问器）、`Sealed Classes`。
* **复杂作用域**：精确识别方法内部的**局部类 (Local Class)** 及匿名内部类。
* **泛型处理**：支持泛型擦除后的 QN 匹配，同时在详细信息中保留完整泛型签名。


* **高性能并发流水线**：采用两阶段（Phase 1: 符号收集，Phase 2: 关系提取）异步处理模型，充分利用多核性能。
* **工业级产物**：输出标准的 **JSON Lines (JSONL)** 格式，易于对接 Neo4j、D3.js 等图数据库或可视化工具。
* **开发友好**：内置 AST 格式化导出工具，助力开发者快速调试 Tree-sitter 查询语句。

---

## 🏗 技术架构

分析器采用两阶段解耦架构，确保了符号解析的准确性：

1. **Phase 1: Collector (定义收集)**
* 扫描项目所有文件。
* 生成唯一的 **Qualified Name (QN)**（例如：`pkg.Class.method(String[])`）。
* 构建全局符号索引（Global Context）。


2. **Phase 2: Extractor (关系提取)**
* 利用 Tree-sitter Query 捕获动作（Call, Create, Use, Cast）。
* **符号消歧**：根据继承链（Inheritance Chain）和导入表（Import Table）自动补全缺失的 QN 路径。
* 建立关联边（Dependency Relations）。



---

## 🚀 快速开始

### 环境要求

* Go 1.22+
* 依赖于 Tree-sitter 及其语言绑定（如 `tree-sitter-java`）

### 安装

```bash
git clone https://github.com/CodMac/go-treesitter-dependency-analyzer.git
cd go-treesitter-dependency-analyzer
go mod tidy
go build -o analyzer main.go

```

### 使用示例

分析一个 Java 项目并将结果保存到 `./out` 目录：

```bash
./analyzer -lang java -path /path/to/your/java_project -jobs 8 -out-dir ./out

```

**命令行参数说明：**

* `-lang`: 分析的语言（目前主打 `java`）。
* `-path`: 源码根目录。
* `-jobs`: 并发工作协程数（默认 4）。
* `-out-dir`: 结果输出目录。
* `-filter`: 选填，用于过滤特定文件（正则）。

---

## 📊 数据模式

### 1. 元素 (Element)

存储于 `element.jsonl`，代表代码中的每一个实体。

| 字段 | 说明 | 示例 |
| :--- | :--- | :--- |
| **Kind** | 类型 | `Class`, `Method`, `Field`, `Interface` |
| **Name** | 短名称 | `UserService` |
| **QualifiedName** | 唯一路径 | `com.service.UserService.getUser(long)` |
| **Path** | 文件相对路径 | `src/main/java/com/service/UserService.java` |
| **Extra** | 扩展信息 | 注解、修饰符、方法签名、是否为 JDK 内置类 |

### 2. 关系 (Relation)

存储于 `relation.jsonl`，代表实体间的连接。

| 关系类型 | 说明 |
| :--- | :--- |
| **CONTAIN** | 包含关系（类包含方法、包包含类） |
| **EXTEND/IMPLEMENT** | 继承与接口实现 |
| **CALL** | 方法调用（含 `super()` 和 `this()`） |
| **CREATE** | 对象实例化（`new` 操作） |
| **USE** | 字段/常量访问 |
| **ANNOTATION** | 类或方法上的注解引用 |
| **RETURN/PARAMETER** | 签名中的类型依赖 |

---

## 🛠 开发与调试

### AST 格式化导出

为了方便调试，解析器支持将 Tree-sitter 的 AST 以格式化的 S-Expression 导出：

1. 在代码中启用 `FormatAST: true`。
2. 分析后会生成 `.java.ast.format` 文件，清晰展示语法树嵌套结构：
```lisp
(class_declaration
  name: (identifier "UserService")
  body: (class_body
    (method_declaration
      name: (identifier "save")
      ...)))

```



### 符号解析策略

本项目内置了 `JavaBuiltinTable`，能够自动识别并链接到 JDK 核心库（如 `java.lang.System`, `java.util.List`）。对于项目内代码，它会沿着 **继承链 (Inheritance Chain)** 向上寻找成员定义，从而精准定位类似 `this.id` 的来源。

---

## 🗺 路线图

* [x] 并发处理流水线。
* [x] Java 17 现代语法支持。
* [x] JSONL 流式导出。
* [ ] 增加对 **Go** 语言的全面支持。
* [ ] 导出到 **DOT** 格式以支持 Graphviz 绘图。
* [ ] 循环依赖检测（Circular Dependency Detection）。

---