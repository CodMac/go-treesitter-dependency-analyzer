package java

import (
	"fmt"

	"path/filepath"

	"strings"

	"github.com/CodMac/go-treesitter-dependency-analyzer/model"

	"github.com/CodMac/go-treesitter-dependency-analyzer/parser"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

type Extractor struct{}

func NewJavaExtractor() *Extractor {

	return &Extractor{}

}

const JavaActionQuery = `

   [

      (method_invocation name: (identifier) @call_target) @call_stmt

      (method_reference (identifier) @ref_target) @ref_stmt

      (object_creation_expression 

          type: [(type_identifier) @create_target_name (generic_type (type_identifier) @create_target_name)]) @create_stmt

      (field_access field: (identifier) @use_field_name) @use_field_stmt

      (cast_expression type: (type_identifier) @cast_type) @cast_stmt

   ]

`

func (e *Extractor) Extract(filePath string, gCtx *model.GlobalContext) ([]*model.DependencyRelation, error) {

	fCtx, ok := gCtx.FileContexts[filePath]

	if !ok {

		return nil, fmt.Errorf("failed to get FileContext: %s", filePath)

	}

	tsLang, err := parser.GetLanguage(model.LangJava)

	if err != nil {

		return nil, err

	}

	relations := make([]*model.DependencyRelation, 0)

	relations = append(relations, e.extractFileBaseRelations(fCtx, gCtx)...)

	relations = append(relations, e.extractStructuralRelations(fCtx, gCtx)...)

	actionRels, err := e.processActionQuery(fCtx, gCtx, tsLang)

	if err != nil {

		return nil, err

	}

	return append(relations, actionRels...), nil

}

// --- 基础关系：Package, Imports ---

func (e *Extractor) extractFileBaseRelations(fCtx *model.FileContext, gCtx *model.GlobalContext) []*model.DependencyRelation {

	rels := make([]*model.DependencyRelation, 0)

	fileElem := &model.CodeElement{

		Kind: model.File, Name: filepath.Base(fCtx.FilePath), QualifiedName: fCtx.FilePath, Path: fCtx.FilePath,
	}

	if fCtx.PackageName != "" {

		pkgElem := &model.CodeElement{Kind: model.Package, Name: fCtx.PackageName, QualifiedName: fCtx.PackageName}

		rels = append(rels, &model.DependencyRelation{Type: model.Contain, Source: pkgElem, Target: fileElem})

	}

	for _, imp := range fCtx.Imports {

		cleanPath := strings.TrimSuffix(imp.RawImportPath, ".*")

		rels = append(rels, &model.DependencyRelation{

			Type: model.Import,

			Source: fileElem,

			Target: e.resolveTargetElement(cleanPath, imp.Kind, fCtx, gCtx),
		})

	}

	return rels

}

// --- 结构化关系：Inheritance, Contains, Annotations, Method Metadata ---

func (e *Extractor) extractStructuralRelations(fCtx *model.FileContext, gCtx *model.GlobalContext) []*model.DependencyRelation {

	rels := make([]*model.DependencyRelation, 0)

	for _, entries := range fCtx.DefinitionsBySN {

		for _, entry := range entries {

			elem := entry.Element

			if elem.Extra == nil {

				continue

			}

			// 1. Annotations

			for _, anno := range elem.Extra.Annotations {

				cleanAnno := e.stripAnnotationArgs(anno)

				rels = append(rels, &model.DependencyRelation{

					Type: model.Annotation,

					Source: elem,

					Target: e.resolveTargetElement(cleanAnno, model.KAnnotation, fCtx, gCtx),
				})

			}

			// 2. Parental Containment

			if entry.ParentQN != "" && entry.ParentQN != fCtx.PackageName {

				if parents, ok := gCtx.DefinitionsByQN[entry.ParentQN]; ok {

					rels = append(rels, &model.DependencyRelation{Type: model.Contain, Source: parents[0].Element, Target: elem})

				}

			}

			// 3. Metadata (Extends/Implements/Throws/Params/Returns)

			e.collectExtraRelations(elem, fCtx, gCtx, &rels)

		}

	}

	return rels

}

func (e *Extractor) collectExtraRelations(elem *model.CodeElement, fCtx *model.FileContext, gCtx *model.GlobalContext, rels *[]*model.DependencyRelation) {

	if elem.Extra == nil {

		return

	}

	// Class/Interface Inheritance

	if ce := elem.Extra.ClassExtra; ce != nil {

		if ce.SuperClass != "" {

			tKind := model.Class

			if elem.Kind == model.Interface {

				tKind = model.Interface

			}

			*rels = append(*rels, &model.DependencyRelation{

				Type: model.Extend,

				Source: elem,

				Target: e.resolveTargetElement(e.cleanTypeName(ce.SuperClass), tKind, fCtx, gCtx),
			})

		}

		for _, imp := range ce.ImplementedInterfaces {

			relType := model.Implement

			if elem.Kind == model.Interface {

				relType = model.Extend

			}

			*rels = append(*rels, &model.DependencyRelation{

				Type: relType,

				Source: elem,

				Target: e.resolveTargetElement(e.cleanTypeName(imp), model.Interface, fCtx, gCtx),
			})

		}

	}

	// Method Metadata

	if me := elem.Extra.MethodExtra; me != nil {

		if me.ReturnType != "" && me.ReturnType != "void" {

			*rels = append(*rels, &model.DependencyRelation{

				Type: model.Return,

				Source: elem,

				Target: e.resolveTargetElement(e.cleanTypeName(me.ReturnType), model.Type, fCtx, gCtx),
			})

		}

		for _, pInfo := range me.Parameters {

			if parts := strings.Fields(pInfo); len(parts) >= 1 {

				*rels = append(*rels, &model.DependencyRelation{

					Type: model.Parameter,

					Source: elem,

					Target: e.resolveTargetElement(e.cleanTypeName(parts[0]), model.Type, fCtx, gCtx),
				})

			}

		}

		for _, tType := range me.ThrowsTypes {

			*rels = append(*rels, &model.DependencyRelation{

				Type: model.Throw,

				Source: elem,

				Target: e.resolveTargetElement(e.cleanTypeName(tType), model.Class, fCtx, gCtx),
			})

		}

	}

}

// --- Action Query: Method Calls, Field Usage, Creation ---

func (e *Extractor) processActionQuery(fCtx *model.FileContext, gCtx *model.GlobalContext, tsLang *sitter.Language) ([]*model.DependencyRelation, error) {

	rels := make([]*model.DependencyRelation, 0)

	q, err := sitter.NewQuery(tsLang, JavaActionQuery)

	if err != nil {

		return nil, err

	}

	defer q.Close()

	qc := sitter.NewQueryCursor()

	matches := qc.Matches(q, fCtx.RootNode, *fCtx.SourceBytes)

	for {

		match := matches.Next()

		if match == nil {

			break

		}

		sourceNode := &match.Captures[0].Node

		sourceElem := e.determineSourceElement(sourceNode, fCtx, gCtx)

		if sourceElem == nil {

			continue

		}

		for _, cap := range match.Captures {

			capName := q.CaptureNames()[cap.Index]

			node := cap.Node

			rawName := node.Utf8Text(*fCtx.SourceBytes)

			var targetElem *model.CodeElement

			var relType model.DependencyType = model.Use

			switch capName {

			case "call_target", "ref_target":

				relType = model.Call

				parentKind := "method_invocation"

				if capName == "ref_target" {

					parentKind = "method_reference"

				}

				prefix := e.getObjectPrefix(&node, parentKind, fCtx)

				targetElem = e.resolveWithPrefix(rawName, prefix, model.Method, sourceElem, fCtx, gCtx)

			case "create_target_name":

				relType = model.Create

				targetElem = e.resolveTargetElement(e.cleanTypeName(rawName), model.Class, fCtx, gCtx)

			case "use_field_name":

				relType = model.Use

				prefix := e.getObjectPrefix(&node, "field_access", fCtx)

				targetElem = e.resolveWithPrefix(rawName, prefix, model.Field, sourceElem, fCtx, gCtx)

			case "cast_type":

				relType = model.Cast

				targetElem = e.resolveTargetElement(e.cleanTypeName(rawName), model.Type, fCtx, gCtx)

			}

			if targetElem != nil {

				rels = append(rels, &model.DependencyRelation{

					Type: relType, Source: sourceElem, Target: targetElem, Location: e.nodeToLocation(&node, fCtx.FilePath),
				})

			}

		}

	}

	return rels, nil

}

// --- Symbol Resolution Core ---

func (e *Extractor) resolveTargetElement(cleanName string, defaultKind model.ElementKind, fCtx *model.FileContext, gCtx *model.GlobalContext) *model.CodeElement {

	// 1. Global Symbol Table

	if entries := gCtx.ResolveSymbol(fCtx, cleanName); len(entries) > 0 {

		found := entries[0].Element

		return &model.CodeElement{Kind: found.Kind, Name: found.Name, QualifiedName: found.QualifiedName, Path: found.Path, Extra: found.Extra}

	}

	// 2. Java Built-in Table

	if builtin := e.resolveFromBuiltin(cleanName); builtin != nil {

		return builtin

	}

	// 3. Dot-separated references (e.g. RetentionPolicy.RUNTIME)

	if strings.Contains(cleanName, ".") {

		parts := strings.Split(cleanName, ".")

		lastPart := parts[len(parts)-1]

		if info, ok := JavaBuiltinTable[lastPart]; ok && strings.Contains(info.QN, parts[len(parts)-2]) {

			return &model.CodeElement{Kind: info.Kind, Name: lastPart, QualifiedName: info.QN}

		}

		// Recursive prefix resolution

		prefixResolved := e.resolveTargetElement(parts[0], model.Unknown, fCtx, gCtx)

		if prefixResolved.QualifiedName != parts[0] {

			return &model.CodeElement{

				Kind: defaultKind, Name: lastPart,

				QualifiedName: prefixResolved.QualifiedName + "." + strings.Join(parts[1:], "."),
			}

		}

	}

	// 4. Implicit java.lang (Capitalized symbols)

	if len(cleanName) > 0 && cleanName[0] >= 'A' && cleanName[0] <= 'Z' {

		if defaultKind == model.Class || defaultKind == model.Type || defaultKind == model.KAnnotation {

			return &model.CodeElement{

				Kind: defaultKind, Name: cleanName, QualifiedName: "java.lang." + cleanName,

				Extra: &model.ElementExtra{ClassExtra: &model.ClassExtra{IsBuiltin: true}},
			}

		}

	}

	return &model.CodeElement{Kind: defaultKind, Name: cleanName, QualifiedName: cleanName}

}

func (e *Extractor) resolveFromBuiltin(name string) *model.CodeElement {

	if info, ok := JavaBuiltinTable[name]; ok {

		elem := &model.CodeElement{Kind: info.Kind, Name: name, QualifiedName: info.QN}

		if info.Kind == model.Class || info.Kind == model.Interface || info.Kind == model.Enum {

			elem.Extra = &model.ElementExtra{ClassExtra: &model.ClassExtra{IsBuiltin: true}}

		}

		return elem

	}

	return nil

}

func (e *Extractor) resolveWithPrefix(name, prefix string, kind model.ElementKind, sourceElem *model.CodeElement, fCtx *model.FileContext, gCtx *model.GlobalContext) *model.CodeElement {

	if prefix == "" {

		return e.resolveTargetElement(name, kind, fCtx, gCtx)

	}

	if prefix == "this" && sourceElem != nil {

		classQN := sourceElem.QualifiedName

		if idx := strings.LastIndex(classQN, "."); idx != -1 {

			classQN = classQN[:idx]

		}

		if resolved := e.resolveInInheritanceChain(classQN, name, kind, gCtx); resolved != nil {

			return resolved

		}

		return e.resolveTargetElement(classQN+"."+name, kind, fCtx, gCtx)

	}

	resolvedPrefix := e.resolveTargetElement(e.cleanTypeName(prefix), model.Variable, fCtx, gCtx)

	if resolvedPrefix.Kind == model.Class {

		if resolved := e.resolveInInheritanceChain(resolvedPrefix.QualifiedName, name, kind, gCtx); resolved != nil {

			return resolved

		}

	}

	fullQN := resolvedPrefix.QualifiedName + "." + name

	if entries := gCtx.ResolveSymbol(fCtx, fullQN); len(entries) > 0 {

		return entries[0].Element

	}

	return &model.CodeElement{Kind: kind, Name: name, QualifiedName: fullQN}

}

func (e *Extractor) resolveInInheritanceChain(classQN, memberName string, kind model.ElementKind, gCtx *model.GlobalContext) *model.CodeElement {

	currQN, visited := classQN, make(map[string]bool)

	for currQN != "" && !visited[currQN] {

		visited[currQN] = true

		targetQN := currQN + "." + memberName

		if defs, ok := gCtx.DefinitionsByQN[targetQN]; ok {

			return defs[0].Element

		}

		defs, ok := gCtx.DefinitionsByQN[currQN]

		if !ok || len(defs) == 0 || defs[0].Element.Extra == nil || defs[0].Element.Extra.ClassExtra == nil {

			break

		}

		rawSuper := defs[0].Element.Extra.ClassExtra.SuperClass

		if rawSuper == "" || rawSuper == "Object" {

			break

		}

		cleanSuper, found := e.cleanTypeName(rawSuper), false

		if _, ok := gCtx.DefinitionsByQN[cleanSuper]; ok {

			currQN, found = cleanSuper, true

		} else {

			for qn := range gCtx.DefinitionsByQN {

				if strings.HasSuffix(qn, "."+cleanSuper) {

					currQN, found = qn, true

					break

				}

			}

		}

		if !found {

			break

		}

	}

	return nil

}

// --- Helpers: AST & String Cleaning ---

func (e *Extractor) getObjectPrefix(node *sitter.Node, parentKind string, fCtx *model.FileContext) string {

	parent := node.Parent()

	for parent != nil && parent.Kind() != parentKind {

		parent = parent.Parent()

	}

	if parent == nil {

		return ""

	}

	if parentKind == "method_invocation" || parentKind == "field_access" {

		if obj := parent.ChildByFieldName("object"); obj != nil {

			return obj.Utf8Text(*fCtx.SourceBytes)

		}

	} else if parentKind == "method_reference" {

		var parts []string

		for i := 0; i < int(parent.ChildCount()); i++ {

			child := parent.Child(uint(i))

			if child.Kind() == "::" {

				break

			}

			parts = append(parts, child.Utf8Text(*fCtx.SourceBytes))

		}

		return strings.Join(parts, "")

	}

	return ""

}

func (e *Extractor) determineSourceElement(n *sitter.Node, fCtx *model.FileContext, gCtx *model.GlobalContext) *model.CodeElement {

	for curr := n.Parent(); curr != nil; curr = curr.Parent() {

		if strings.Contains(curr.Kind(), "declaration") {

			if nameNode := curr.ChildByFieldName("name"); nameNode != nil {

				name := nameNode.Utf8Text(*fCtx.SourceBytes)

				for _, entry := range gCtx.ResolveSymbol(fCtx, name) {

					if int(curr.StartPosition().Row)+1 == entry.Element.Location.StartLine {

						return entry.Element

					}

				}

			}

		}

	}

	return nil

}

func (e *Extractor) stripAnnotationArgs(name string) string {

	name = strings.TrimPrefix(strings.TrimSpace(name), "@")

	if idx := strings.Index(name, "("); idx != -1 {

		return name[:idx]

	}

	return name

}

func (e *Extractor) cleanTypeName(name string) string {

	name = strings.TrimPrefix(strings.TrimSpace(name), "@")

	if idx := strings.Index(name, " extends "); idx != -1 {

		name = name[idx+len(" extends "):]

	}

	if idx := strings.Index(name, "<"); idx != -1 {

		name = name[:idx]

	}

	name = strings.TrimSuffix(name, "[]")

	name = strings.TrimSuffix(name, "...")

	return strings.TrimSpace(name)

}

func (e *Extractor) nodeToLocation(n *sitter.Node, fp string) *model.Location {

	if n == nil {

		return nil

	}

	return &model.Location{

		FilePath: fp, StartLine: int(n.StartPosition().Row) + 1, EndLine: int(n.EndPosition().Row) + 1,

		StartColumn: int(n.StartPosition().Column), EndColumn: int(n.EndPosition().Column),
	}

}

// --- Java 内置符号表 ---

var JavaBuiltinTable = map[string]struct {
	QN string

	Kind model.ElementKind
}{

	// === java.lang 核心类 (默认隐式导入) ===

	"String": {"java.lang.String", model.Class},

	"Object": {"java.lang.Object", model.Class},

	"System": {"java.lang.System", model.Class},

	"Integer": {"java.lang.Integer", model.Class},

	"Long": {"java.lang.Long", model.Class},

	"Double": {"java.lang.Double", model.Class},

	"Float": {"java.lang.Float", model.Class},

	"Boolean": {"java.lang.Boolean", model.Class},

	"Byte": {"java.lang.Byte", model.Class},

	"Character": {"java.lang.Character", model.Class},

	"Short": {"java.lang.Short", model.Class},

	"Void": {"java.lang.Void", model.Class},

	"Number": {"java.lang.Number", model.Class},

	"Math": {"java.lang.Math", model.Class},

	"Class": {"java.lang.Class", model.Class},

	"ClassLoader": {"java.lang.ClassLoader", model.Class},

	"Thread": {"java.lang.Thread", model.Class},

	"ThreadGroup": {"java.lang.ThreadGroup", model.Class},

	"ThreadLocal": {"java.lang.ThreadLocal", model.Class},

	"StringBuilder": {"java.lang.StringBuilder", model.Class},

	"StringBuffer": {"java.lang.StringBuffer", model.Class},

	"Enum": {"java.lang.Enum", model.Class},

	"Throwable": {"java.lang.Throwable", model.Class},

	"Exception": {"java.lang.Exception", model.Class},

	"RuntimeException": {"java.lang.RuntimeException", model.Class},

	"Error": {"java.lang.Error", model.Class},

	"StackTraceElement": {"java.lang.StackTraceElement", model.Class},

	"Iterable": {"java.lang.Iterable", model.Interface},

	"AutoCloseable": {"java.lang.AutoCloseable", model.Interface},

	"Runnable": {"java.lang.Runnable", model.Interface},

	"Comparable": {"java.lang.Comparable", model.Interface},

	"CharSequence": {"java.lang.CharSequence", model.Interface},

	// === java.lang 常用异常 ===

	"NullPointerException": {"java.lang.NullPointerException", model.Class},

	"IllegalArgumentException": {"java.lang.IllegalArgumentException", model.Class},

	"IllegalStateException": {"java.lang.IllegalStateException", model.Class},

	"IndexOutOfBoundsException": {"java.lang.IndexOutOfBoundsException", model.Class},

	"UnsupportedOperationException": {"java.lang.UnsupportedOperationException", model.Class},

	// === java.lang.annotation 核心元注解与枚举 ===

	"Retention": {"java.lang.annotation.Retention", model.KAnnotation},

	"Target": {"java.lang.annotation.Target", model.KAnnotation},

	"Documented": {"java.lang.annotation.Documented", model.KAnnotation},

	"Inherited": {"java.lang.annotation.Inherited", model.KAnnotation},

	"Native": {"java.lang.annotation.Native", model.KAnnotation},

	"Repeatable": {"java.lang.annotation.Repeatable", model.KAnnotation},

	// 元注解使用的枚举类型

	"RetentionPolicy": {"java.lang.annotation.RetentionPolicy", model.Enum},

	"ElementType": {"java.lang.annotation.ElementType", model.Enum},

	// 常见的枚举常量 (支持在注解参数中直接解析)

	"RUNTIME": {"java.lang.annotation.RetentionPolicy.RUNTIME", model.Field},

	"SOURCE": {"java.lang.annotation.RetentionPolicy.SOURCE", model.Field},

	"CLASS": {"java.lang.annotation.RetentionPolicy.CLASS", model.Field},

	"TYPE": {"java.lang.annotation.ElementType.TYPE", model.Field},

	"METHOD": {"java.lang.annotation.ElementType.METHOD", model.Field},

	"FIELD": {"java.lang.annotation.ElementType.FIELD", model.Field},

	"PARAMETER": {"java.lang.annotation.ElementType.PARAMETER", model.Field},

	// === java.util 集合框架 ===

	"Collection": {"java.util.Collection", model.Interface},

	"List": {"java.util.List", model.Interface},

	"ArrayList": {"java.util.ArrayList", model.Class},

	"LinkedList": {"java.util.LinkedList", model.Class},

	"Set": {"java.util.Set", model.Interface},

	"HashSet": {"java.util.HashSet", model.Class},

	"TreeSet": {"java.util.TreeSet", model.Class},

	"Map": {"java.util.Map", model.Interface},

	"HashMap": {"java.util.HashMap", model.Class},

	"TreeMap": {"java.util.TreeMap", model.Class},

	"LinkedHashMap": {"java.util.LinkedHashMap", model.Class},

	"Iterator": {"java.util.Iterator", model.Interface},

	"Optional": {"java.util.Optional", model.Class},

	"Arrays": {"java.util.Arrays", model.Class},

	"Collections": {"java.util.Collections", model.Class},

	"UUID": {"java.util.UUID", model.Class},

	"Date": {"java.util.Date", model.Class},

	"Objects": {"java.util.Objects", model.Class},

	"Scanner": {"java.util.Scanner", model.Class},

	"Properties": {"java.util.Properties", model.Class},

	// === java.util.stream & function (现代 Java 高频) ===

	"Stream": {"java.util.stream.Stream", model.Interface},

	"Collectors": {"java.util.stream.Collectors", model.Class},

	"Function": {"java.util.function.Function", model.Interface},

	"BiFunction": {"java.util.function.BiFunction", model.Interface},

	"Consumer": {"java.util.function.Consumer", model.Interface},

	"Predicate": {"java.util.function.Predicate", model.Interface},

	"Supplier": {"java.util.function.Supplier", model.Interface},

	// === java.time (JSR-310 现代日期) ===

	"LocalDate": {"java.time.LocalDate", model.Class},

	"LocalTime": {"java.time.LocalTime", model.Class},

	"LocalDateTime": {"java.time.LocalDateTime", model.Class},

	"ZonedDateTime": {"java.time.ZonedDateTime", model.Class},

	"Duration": {"java.time.Duration", model.Class},

	"Instant": {"java.time.Instant", model.Class},

	// === java.io & java.nio ===

	"InputStream": {"java.io.InputStream", model.Class},

	"OutputStream": {"java.io.OutputStream", model.Class},

	"File": {"java.io.File", model.Class},

	"Serializable": {"java.io.Serializable", model.Interface},

	"Path": {"java.nio.file.Path", model.Interface},

	"Paths": {"java.nio.file.Paths", model.Class},

	"Files": {"java.nio.file.Files", model.Class},

	// === java.util.concurrent ===

	"Executor": {"java.util.concurrent.Executor", model.Interface},

	"ExecutorService": {"java.util.concurrent.ExecutorService", model.Interface},

	"Executors": {"java.util.concurrent.Executors", model.Class},

	"Future": {"java.util.concurrent.Future", model.Interface},

	"CompletableFuture": {"java.util.concurrent.CompletableFuture", model.Class},

	"ConcurrentHashMap": {"java.util.concurrent.ConcurrentHashMap", model.Class},

	"TimeUnit": {"java.util.concurrent.TimeUnit", model.Enum},

	// === 静态字段与内置对象 ===

	"out": {"java.lang.System.out", model.Field},

	"err": {"java.lang.System.err", model.Field},

	"in": {"java.lang.System.in", model.Field},
}
