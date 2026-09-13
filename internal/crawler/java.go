package crawler

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/java"

	"github.com/anurag/nexus/model"
)

type JavaParser struct {
	lang *sitter.Language
}

func NewJavaParser() *JavaParser {
	return &JavaParser{lang: java.GetLanguage()}
}

func (jp *JavaParser) Platform() model.Platform { return model.Java }

func (jp *JavaParser) Extensions() []string { return []string{".java"} }

func (jp *JavaParser) ParseFile(path string, content []byte) ([]model.Unit, error) {
	p := sitter.NewParser()
	p.SetLanguage(jp.lang)
	tree, err := p.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	root := tree.RootNode()
	imports := jp.extractImports(root, content)

	var units []model.Unit
	jp.walkNode(root, content, path, &units)

	isTest := strings.Contains(path, "src/test/")
	source := string(content)
	for i := range units {
		units[i].Imports = imports
		if !isTest {
			units[i].ThrowSites = extractThrowSites(source, imports, units[i].Methods)
		}
	}
	return units, nil
}

func (jp *JavaParser) extractImports(root *sitter.Node, src []byte) []string {
	var imports []string
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() == "import_declaration" {
			imp := child.Content(src)
			imp = strings.TrimPrefix(imp, "import ")
			imp = strings.TrimSuffix(imp, ";")
			imp = strings.TrimSpace(imp)
			if imp != "" {
				imports = append(imports, imp)
			}
		}
	}
	return imports
}

func (jp *JavaParser) walkNode(node *sitter.Node, src []byte, file string, units *[]model.Unit) {
	switch node.Type() {
	case "class_declaration", "interface_declaration", "enum_declaration":
		unit := jp.extractClass(node, src, file)
		*units = append(*units, unit)
		return // don't recurse into inner classes for now
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		jp.walkNode(node.Child(i), src, file, units)
	}
}

func (jp *JavaParser) extractClass(node *sitter.Node, src []byte, file string) model.Unit {
	unit := model.Unit{
		Type: node.Type(),
		File: file,
		Line: int(node.StartPoint().Row) + 1,
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "identifier":
			if unit.Name == "" {
				unit.Name = child.Content(src)
			}
		case "modifiers":
			jp.extractAnnotations(child, src, &unit)
		case "superclass":
			for j := 0; j < int(child.ChildCount()); j++ {
				sc := child.Child(j)
				if sc.Type() == "type_identifier" || sc.Type() == "generic_type" {
					unit.Extends = sc.Content(src)
				}
			}
		case "super_interfaces":
			jp.extractInterfaces(child, src, &unit)
		case "extends_interfaces":
			jp.extractInterfaces(child, src, &unit)
		case "class_body", "interface_body", "enum_body":
			jp.extractFields(child, src, &unit)
			jp.extractMethods(child, src, &unit)
		}
	}

	unit.Stereotype = detectStereotype(unit)
	return unit
}

func (jp *JavaParser) extractAnnotations(node *sitter.Node, src []byte, unit *model.Unit) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "marker_annotation" || child.Type() == "annotation" {
			ann := child.Content(src)
			unit.Annotations = append(unit.Annotations, ann)
			if strings.Contains(ann, "KafkaListener") {
				topic := extractAnnotationValue(ann, "topics")
				if topic != "" {
					unit.KafkaConsumes = append(unit.KafkaConsumes, topic)
				}
			}
			if strings.Contains(ann, "ConfigurationProperties") {
				prefix := extractAnnotationValue(ann, "prefix")
				if prefix == "" {
					prefix = extractAnnotationValue(ann, "value")
				}
				if prefix != "" {
					unit.ConfigRefs = append(unit.ConfigRefs, prefix)
				}
			}
		}
	}
}

func (jp *JavaParser) extractInterfaces(node *sitter.Node, src []byte, unit *model.Unit) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_identifier" || child.Type() == "generic_type" {
			unit.Implements = append(unit.Implements, child.Content(src))
		} else if child.Type() == "type_list" {
			for j := 0; j < int(child.ChildCount()); j++ {
				tc := child.Child(j)
				if tc.Type() == "type_identifier" || tc.Type() == "generic_type" {
					unit.Implements = append(unit.Implements, tc.Content(src))
				}
			}
		}
	}
}

func (jp *JavaParser) extractFields(bodyNode *sitter.Node, src []byte, unit *model.Unit) {
	for i := 0; i < int(bodyNode.ChildCount()); i++ {
		child := bodyNode.Child(i)
		if child.Type() != "field_declaration" {
			continue
		}
		fieldText := child.Content(src)

		// Extract @Value config refs
		if strings.Contains(fieldText, "@Value") {
			start := strings.Index(fieldText, "${")
			if start >= 0 {
				end := strings.Index(fieldText[start:], "}")
				if end > 0 {
					ref := fieldText[start+2 : start+end]
					if colon := strings.Index(ref, ":"); colon >= 0 {
						ref = ref[:colon]
					}
					unit.ConfigRefs = append(unit.ConfigRefs, ref)
				}
			}
		}

		// Extract field name, type, and annotations for ALL fields
		var fieldName, fieldType string
		var fieldAnnotations []string
		for j := 0; j < int(child.ChildCount()); j++ {
			fc := child.Child(j)
			switch fc.Type() {
			case "modifiers":
				for k := 0; k < int(fc.ChildCount()); k++ {
					mod := fc.Child(k)
					if mod.Type() == "marker_annotation" || mod.Type() == "annotation" {
						fieldAnnotations = append(fieldAnnotations, mod.Content(src))
					}
				}
			case "type_identifier", "generic_type", "boolean_type",
				"integral_type", "floating_point_type", "void_type", "array_type":
				if fieldType == "" {
					fieldType = fc.Content(src)
				}
			case "variable_declarator":
				for k := 0; k < int(fc.ChildCount()); k++ {
					vc := fc.Child(k)
					if vc.Type() == "identifier" && fieldName == "" {
						fieldName = vc.Content(src)
					}
				}
			}
		}

		if fieldName != "" && fieldType != "" {
			unit.Fields = append(unit.Fields, model.Field{
				Name:        fieldName,
				Type:        fieldType,
				Annotations: fieldAnnotations,
			})
		}

		// Extract private final dependencies (existing logic)
		isFinal := strings.Contains(fieldText, "final ")
		isPrivate := strings.Contains(fieldText, "private ")
		if isFinal && isPrivate {
			for j := 0; j < int(child.ChildCount()); j++ {
				fc := child.Child(j)
				if fc.Type() == "type_identifier" || fc.Type() == "generic_type" {
					typeName := fc.Content(src)
					if idx := strings.LastIndex(typeName, "<"); idx >= 0 {
						typeName = typeName[:idx]
					}
					if !isJdkType(typeName) {
						unit.Dependencies = append(unit.Dependencies, typeName)
					}
				}
			}
		}
	}
}

func isJdkType(name string) bool {
	switch name {
	case "String", "Integer", "Long", "Boolean", "Double", "Float",
		"List", "Map", "Set", "Optional", "Duration",
		"ObjectMapper", "Logger":
		return true
	}
	return false
}

func (jp *JavaParser) extractMethods(bodyNode *sitter.Node, src []byte, unit *model.Unit) {
	for i := 0; i < int(bodyNode.ChildCount()); i++ {
		child := bodyNode.Child(i)
		if child.Type() != "method_declaration" {
			continue
		}
		m := model.Method{
			Line: int(child.StartPoint().Row) + 1,
		}
		for j := 0; j < int(child.ChildCount()); j++ {
			mc := child.Child(j)
			switch mc.Type() {
			case "identifier":
				if m.Name == "" {
					m.Name = mc.Content(src)
				}
			case "type_identifier", "void_type", "generic_type":
				if m.ReturnType == "" {
					m.ReturnType = mc.Content(src)
				}
			case "modifiers":
				jp.extractMethodAnnotations(mc, src, &m, unit)
			case "formal_parameters":
				m.Parameters = extractParams(mc, src)
			case "block":
				jp.extractMethodBodySignals(mc, src, unit)
				m.Calls = jp.extractMethodCalls(mc, src)
			}
		}
		unit.Methods = append(unit.Methods, m)
	}
}

func (jp *JavaParser) extractMethodAnnotations(node *sitter.Node, src []byte, m *model.Method, unit *model.Unit) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() != "marker_annotation" && child.Type() != "annotation" {
			continue
		}
		ann := child.Content(src)
		m.Annotations = append(m.Annotations, ann)

		switch {
		case strings.Contains(ann, "GetMapping"):
			m.HTTPMethod = "GET"
			m.HTTPPath = extractAnnotationValue(ann, "value")
			if m.HTTPPath == "" {
				m.HTTPPath = extractAnnotationSingleValue(ann)
			}
			unit.Endpoints = append(unit.Endpoints, "GET "+m.HTTPPath)
		case strings.Contains(ann, "PostMapping"):
			m.HTTPMethod = "POST"
			m.HTTPPath = extractAnnotationValue(ann, "value")
			if m.HTTPPath == "" {
				m.HTTPPath = extractAnnotationSingleValue(ann)
			}
			unit.Endpoints = append(unit.Endpoints, "POST "+m.HTTPPath)
		case strings.Contains(ann, "PutMapping"):
			m.HTTPMethod = "PUT"
			m.HTTPPath = extractAnnotationValue(ann, "value")
			if m.HTTPPath == "" {
				m.HTTPPath = extractAnnotationSingleValue(ann)
			}
			unit.Endpoints = append(unit.Endpoints, "PUT "+m.HTTPPath)
		case strings.Contains(ann, "DeleteMapping"):
			m.HTTPMethod = "DELETE"
			m.HTTPPath = extractAnnotationValue(ann, "value")
			if m.HTTPPath == "" {
				m.HTTPPath = extractAnnotationSingleValue(ann)
			}
			unit.Endpoints = append(unit.Endpoints, "DELETE "+m.HTTPPath)
		case strings.Contains(ann, "RequestMapping"):
			m.HTTPMethod = extractAnnotationValue(ann, "method")
			m.HTTPPath = extractAnnotationValue(ann, "value")
			if m.HTTPPath == "" {
				m.HTTPPath = extractAnnotationSingleValue(ann)
			}
			if m.HTTPMethod != "" {
				unit.Endpoints = append(unit.Endpoints, m.HTTPMethod+" "+m.HTTPPath)
			}
		case strings.Contains(ann, "KafkaListener"):
			topic := extractAnnotationValue(ann, "topics")
			if topic != "" {
				unit.KafkaConsumes = append(unit.KafkaConsumes, topic)
			}
		}
	}
}

func (jp *JavaParser) extractMethodBodySignals(bodyNode *sitter.Node, src []byte, unit *model.Unit) {
	body := bodyNode.Content(src)
	if strings.Contains(body, "kafkaTemplate.send") {
		for _, line := range strings.Split(body, "\n") {
			if strings.Contains(line, "kafkaTemplate.send") {
				topic := extractStringArg(line)
				if topic != "" {
					unit.KafkaProduces = append(unit.KafkaProduces, topic)
				}
			}
		}
	}
	if strings.Contains(body, ".uri(") || strings.Contains(body, "WebClient") {
		for _, line := range strings.Split(body, "\n") {
			if strings.Contains(line, ".uri(") {
				uri := extractStringArg(line)
				if uri != "" {
					unit.ApiCalls = append(unit.ApiCalls, uri)
				}
			}
		}
	}
}

func (jp *JavaParser) extractMethodCalls(node *sitter.Node, src []byte) []model.MethodCall {
	var calls []model.MethodCall
	seen := map[string]bool{}
	jp.walkForMethodInvocations(node, src, &calls, seen)
	return calls
}

func (jp *JavaParser) walkForMethodInvocations(node *sitter.Node, src []byte, calls *[]model.MethodCall, seen map[string]bool) {
	if node.Type() == "method_invocation" {
		var target, method string
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			switch child.Type() {
			case "identifier":
				if target == "" && method == "" {
					method = child.Content(src)
				} else if method != "" {
					method = child.Content(src)
				}
			case "field_access":
				target = child.Content(src)
			}
		}
		if node.ChildCount() >= 3 {
			first := node.Child(0)
			dot := node.Child(1)
			second := node.Child(2)
			if dot.Type() == "." {
				target = first.Content(src)
				method = second.Content(src)
			}
		}
		if target != "" && method != "" {
			key := target + "." + method
			if !seen[key] {
				seen[key] = true
				*calls = append(*calls, model.MethodCall{Target: target, Method: method})
			}
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		jp.walkForMethodInvocations(node.Child(i), src, calls, seen)
	}
}

// detectStereotype infers a Spring stereotype from class-level annotations
// and naming conventions.
func detectStereotype(unit model.Unit) string {
	for _, ann := range unit.Annotations {
		switch {
		case strings.Contains(ann, "RestController"):
			return "rest_controller"
		case strings.Contains(ann, "Controller"):
			return "controller"
		case strings.Contains(ann, "Service"):
			return "service"
		case strings.Contains(ann, "Repository"):
			return "repository"
		case strings.Contains(ann, "Configuration"):
			return "configuration"
		case strings.Contains(ann, "Component"):
			return "component"
		}
	}
	if strings.HasSuffix(unit.Name, "Service") || strings.HasSuffix(unit.Name, "ServiceImpl") {
		return "service"
	}
	if strings.HasSuffix(unit.Name, "Repository") {
		return "repository"
	}
	if strings.HasSuffix(unit.Name, "Config") || strings.HasSuffix(unit.Name, "Configuration") {
		return "configuration"
	}
	return ""
}

// --- annotation / string extraction helpers ---

func extractAnnotationValue(ann, key string) string {
	idx := strings.Index(ann, key+"=")
	if idx < 0 {
		idx = strings.Index(ann, key+" =")
	}
	if idx < 0 {
		return ""
	}
	rest := ann[idx+len(key)+1:]
	rest = strings.TrimLeft(rest, " =")
	return extractQuotedString(rest)
}

func extractAnnotationSingleValue(ann string) string {
	start := strings.Index(ann, "(")
	if start < 0 {
		return ""
	}
	inner := ann[start+1:]
	end := strings.Index(inner, ")")
	if end < 0 {
		return ""
	}
	inner = strings.TrimSpace(inner[:end])
	if strings.Contains(inner, "=") {
		return "" // has named params, not a single value
	}
	return strings.Trim(inner, "\"")
}

func extractQuotedString(s string) string {
	start := strings.IndexByte(s, '"')
	if start < 0 {
		return ""
	}
	end := strings.IndexByte(s[start+1:], '"')
	if end < 0 {
		return ""
	}
	return s[start+1 : start+1+end]
}

func extractParams(node *sitter.Node, src []byte) []string {
	var params []string
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "formal_parameter" {
			params = append(params, child.Content(src))
		}
	}
	return params
}
