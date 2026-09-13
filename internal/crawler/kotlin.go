package crawler

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/kotlin"

	"github.com/anurag/nexus/model"
)

type KotlinParser struct {
	lang *sitter.Language
}

func NewKotlinParser() *KotlinParser {
	return &KotlinParser{lang: kotlin.GetLanguage()}
}

func (kp *KotlinParser) Platform() model.Platform { return model.Kotlin }

func (kp *KotlinParser) Extensions() []string { return []string{".kt", ".kts"} }

func (kp *KotlinParser) ParseFile(path string, content []byte) ([]model.Unit, error) {
	p := sitter.NewParser()
	p.SetLanguage(kp.lang)
	tree, err := p.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	root := tree.RootNode()

	var imports []string
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() == "import_list" {
			imports = kp.extractImports(child, content)
		}
	}

	var units []model.Unit
	kp.walkNode(root, content, path, &units)

	if len(imports) > 0 {
		for i := range units {
			units[i].Imports = imports
		}
	}

	kp.extractSignals(content, &units)
	return units, nil
}

func (kp *KotlinParser) walkNode(node *sitter.Node, src []byte, file string, units *[]model.Unit) {
	switch node.Type() {
	case "class_declaration":
		unit := kp.extractClass(node, src, file)
		*units = append(*units, unit)
		return
	case "object_declaration":
		unit := kp.extractObject(node, src, file)
		*units = append(*units, unit)
		return
	case "function_declaration":
		unit := kp.extractTopLevelFunction(node, src, file)
		if unit.Name != "" {
			*units = append(*units, unit)
		}
		return
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		kp.walkNode(node.Child(i), src, file, units)
	}
}

func (kp *KotlinParser) extractClass(node *sitter.Node, src []byte, file string) model.Unit {
	unit := model.Unit{
		Type: "class",
		File: file,
		Line: int(node.StartPoint().Row) + 1,
	}

	var superTypes []string

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "type_identifier":
			if unit.Name == "" {
				unit.Name = child.Content(src)
			}
		case "modifiers":
			kp.extractAnnotations(child, src, &unit)
		case "delegation_specifier":
			st := kp.extractSuperType(child, src)
			if st != "" {
				superTypes = append(superTypes, st)
			}
		case "class_body":
			kp.extractFields(child, src, &unit)
			kp.extractMethods(child, src, &unit)
		case "primary_constructor":
			kp.extractConstructorParams(child, src, &unit)
		}
	}

	unit.Stereotype = detectKotlinStereotype(unit, superTypes)
	return unit
}

func (kp *KotlinParser) extractObject(node *sitter.Node, src []byte, file string) model.Unit {
	unit := model.Unit{
		Type: "object",
		File: file,
		Line: int(node.StartPoint().Row) + 1,
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "type_identifier":
			if unit.Name == "" {
				unit.Name = child.Content(src)
			}
		case "modifiers":
			kp.extractAnnotations(child, src, &unit)
		case "class_body":
			kp.extractMethods(child, src, &unit)
		}
	}
	return unit
}

func (kp *KotlinParser) extractTopLevelFunction(node *sitter.Node, src []byte, file string) model.Unit {
	unit := model.Unit{
		Type: "function",
		File: file,
		Line: int(node.StartPoint().Row) + 1,
	}

	m := model.Method{Line: unit.Line}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "simple_identifier":
			if unit.Name == "" {
				unit.Name = child.Content(src)
				m.Name = unit.Name
			}
		case "modifiers":
			kp.extractAnnotations(child, src, &unit)
			kp.extractMethodAnnotations(child, src, &m, &unit)
		case "function_value_parameters":
			m.Parameters = kp.extractFunctionParams(child, src)
		case "user_type":
			if m.ReturnType == "" {
				m.ReturnType = child.Content(src)
			}
		}
	}

	for _, ann := range unit.Annotations {
		if strings.Contains(ann, "Composable") {
			unit.Stereotype = "composable_screen"
		}
	}

	unit.Methods = append(unit.Methods, m)
	return unit
}

func (kp *KotlinParser) extractAnnotations(node *sitter.Node, src []byte, unit *model.Unit) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "annotation" {
			ann := child.Content(src)
			unit.Annotations = append(unit.Annotations, ann)

			if strings.Contains(ann, "Entity") {
				tableName := extractAnnotationValue(ann, "tableName")
				if tableName != "" {
					unit.ConfigRefs = append(unit.ConfigRefs, "table:"+tableName)
				}
			}
		}
	}
}

func (kp *KotlinParser) extractSuperType(node *sitter.Node, src []byte) string {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "constructor_invocation" || child.Type() == "user_type" {
			for j := 0; j < int(child.ChildCount()); j++ {
				gc := child.Child(j)
				if gc.Type() == "user_type" || gc.Type() == "type_identifier" {
					return gc.Content(src)
				}
			}
		}
	}
	return ""
}

func (kp *KotlinParser) extractFields(bodyNode *sitter.Node, src []byte, unit *model.Unit) {
	for i := 0; i < int(bodyNode.ChildCount()); i++ {
		child := bodyNode.Child(i)
		if child.Type() != "property_declaration" {
			continue
		}
		propText := child.Content(src)

		if strings.Contains(propText, "@Inject") {
			for j := 0; j < int(child.ChildCount()); j++ {
				pc := child.Child(j)
				if pc.Type() == "user_type" {
					dep := pc.Content(src)
					if idx := strings.Index(dep, "<"); idx >= 0 {
						dep = dep[:idx]
					}
					if !isKotlinStdType(dep) {
						unit.Dependencies = append(unit.Dependencies, dep)
					}
				}
			}
		}

		isVal := strings.Contains(propText, "val ")
		isPrivate := strings.Contains(propText, "private ")
		if isVal && isPrivate {
			for j := 0; j < int(child.ChildCount()); j++ {
				pc := child.Child(j)
				if pc.Type() == "user_type" {
					dep := pc.Content(src)
					if idx := strings.Index(dep, "<"); idx >= 0 {
						dep = dep[:idx]
					}
					if !isKotlinStdType(dep) {
						unit.Dependencies = append(unit.Dependencies, dep)
					}
				}
			}
		}
	}
}

func isKotlinStdType(name string) bool {
	switch name {
	case "String", "Int", "Long", "Boolean", "Double", "Float",
		"List", "Map", "Set", "MutableList", "MutableMap", "MutableSet",
		"Context", "Application", "Activity", "Fragment",
		"CoroutineScope", "Flow", "StateFlow", "MutableStateFlow",
		"LiveData", "MutableLiveData", "ViewModel":
		return true
	}
	return false
}

func (kp *KotlinParser) extractMethods(bodyNode *sitter.Node, src []byte, unit *model.Unit) {
	for i := 0; i < int(bodyNode.ChildCount()); i++ {
		child := bodyNode.Child(i)
		if child.Type() != "function_declaration" {
			continue
		}

		m := model.Method{
			Line: int(child.StartPoint().Row) + 1,
		}

		for j := 0; j < int(child.ChildCount()); j++ {
			mc := child.Child(j)
			switch mc.Type() {
			case "simple_identifier":
				if m.Name == "" {
					m.Name = mc.Content(src)
				}
			case "user_type":
				if m.ReturnType == "" {
					m.ReturnType = mc.Content(src)
				}
			case "modifiers":
				kp.extractMethodAnnotations(mc, src, &m, unit)
			case "function_value_parameters":
				m.Parameters = kp.extractFunctionParams(mc, src)
			case "function_body":
				kp.extractMethodBodySignals(mc, src, unit)
			}
		}
		unit.Methods = append(unit.Methods, m)
	}
}

func (kp *KotlinParser) extractMethodAnnotations(node *sitter.Node, src []byte, m *model.Method, unit *model.Unit) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() != "annotation" {
			continue
		}
		ann := child.Content(src)
		m.Annotations = append(m.Annotations, ann)

		annUpper := strings.ToUpper(ann)
		switch {
		case strings.Contains(annUpper, "@GET"):
			m.HTTPMethod = "GET"
			m.HTTPPath = extractAnnotationSingleValue(ann)
			if m.HTTPPath != "" {
				unit.Endpoints = append(unit.Endpoints, "GET "+m.HTTPPath)
			}
		case strings.Contains(annUpper, "@POST"):
			m.HTTPMethod = "POST"
			m.HTTPPath = extractAnnotationSingleValue(ann)
			if m.HTTPPath != "" {
				unit.Endpoints = append(unit.Endpoints, "POST "+m.HTTPPath)
			}
		case strings.Contains(annUpper, "@PUT"):
			m.HTTPMethod = "PUT"
			m.HTTPPath = extractAnnotationSingleValue(ann)
			if m.HTTPPath != "" {
				unit.Endpoints = append(unit.Endpoints, "PUT "+m.HTTPPath)
			}
		case strings.Contains(annUpper, "@DELETE"):
			m.HTTPMethod = "DELETE"
			m.HTTPPath = extractAnnotationSingleValue(ann)
			if m.HTTPPath != "" {
				unit.Endpoints = append(unit.Endpoints, "DELETE "+m.HTTPPath)
			}
		case strings.Contains(annUpper, "@PATCH"):
			m.HTTPMethod = "PATCH"
			m.HTTPPath = extractAnnotationSingleValue(ann)
			if m.HTTPPath != "" {
				unit.Endpoints = append(unit.Endpoints, "PATCH "+m.HTTPPath)
			}
		}
	}
}

func (kp *KotlinParser) extractConstructorParams(node *sitter.Node, src []byte, unit *model.Unit) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "modifiers" {
			kp.extractAnnotations(child, src, unit)
		}
		if child.Type() == "class_parameter" {
			for j := 0; j < int(child.ChildCount()); j++ {
				gc := child.Child(j)
				if gc.Type() == "user_type" {
					dep := gc.Content(src)
					unit.Dependencies = append(unit.Dependencies, dep)
				}
			}
		}
	}
}

func (kp *KotlinParser) extractFunctionParams(node *sitter.Node, src []byte) []string {
	var params []string
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "parameter" {
			params = append(params, child.Content(src))
		}
	}
	return params
}

func (kp *KotlinParser) extractImports(node *sitter.Node, src []byte) []string {
	var imports []string
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "import_header" {
			imports = append(imports, child.Content(src))
		}
	}
	return imports
}

func (kp *KotlinParser) extractMethodBodySignals(bodyNode *sitter.Node, src []byte, unit *model.Unit) {
	body := bodyNode.Content(src)
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, ".url(") || strings.Contains(trimmed, "Request.Builder") {
			url := extractStringArg(trimmed)
			if url != "" {
				unit.ApiCalls = append(unit.ApiCalls, url)
			}
		}
		if strings.Contains(trimmed, "retrofit") || strings.Contains(trimmed, "Retrofit") {
			url := extractStringArg(trimmed)
			if url != "" && (strings.HasPrefix(url, "/") || strings.HasPrefix(url, "http")) {
				unit.ApiCalls = append(unit.ApiCalls, url)
			}
		}
	}
}

func (kp *KotlinParser) extractSignals(content []byte, units *[]model.Unit) {
	src := string(content)
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "BuildConfig.") {
			parts := strings.SplitN(trimmed, "BuildConfig.", 2)
			if len(parts) == 2 {
				key := strings.SplitN(parts[1], " ", 2)[0]
				key = strings.TrimRight(key, ",);")
				if key != "" && len(*units) > 0 {
					(*units)[0].ConfigRefs = append((*units)[0].ConfigRefs, "BuildConfig."+key)
				}
			}
		}
		if strings.Contains(trimmed, "R.string.") {
			idx := strings.Index(trimmed, "R.string.")
			rest := trimmed[idx+9:]
			key := strings.SplitN(rest, " ", 2)[0]
			key = strings.TrimRight(key, ",);")
			if key != "" && len(*units) > 0 {
				(*units)[0].ConfigRefs = append((*units)[0].ConfigRefs, "R.string."+key)
			}
		}
	}
}

func detectKotlinStereotype(unit model.Unit, superTypes []string) string {
	for _, ann := range unit.Annotations {
		switch {
		case strings.Contains(ann, "HiltViewModel"):
			return "viewmodel"
		case strings.Contains(ann, "Entity"):
			return "entity"
		case strings.Contains(ann, "Dao"):
			return "repository"
		case strings.Contains(ann, "Module") && strings.Contains(ann, "InstallIn"):
			return "di_module"
		case strings.Contains(ann, "Module"):
			return "di_module"
		case strings.Contains(ann, "Singleton"):
			return "service"
		case strings.Contains(ann, "Composable"):
			return "composable_screen"
		}
	}

	for _, st := range superTypes {
		switch {
		case st == "Activity" || st == "ComponentActivity" || st == "AppCompatActivity":
			return "activity"
		case st == "Fragment":
			return "fragment"
		case st == "ViewModel" || st == "AndroidViewModel":
			return "viewmodel"
		case st == "BroadcastReceiver":
			return "receiver"
		case st == "IntentService" || strings.HasSuffix(st, "Service"):
			return "service"
		}
	}

	if len(unit.Endpoints) > 0 {
		return "api_service"
	}

	nameLower := strings.ToLower(unit.Name)
	if strings.HasSuffix(nameLower, "repository") || strings.HasSuffix(nameLower, "repo") {
		return "repository"
	}
	if strings.HasSuffix(nameLower, "viewmodel") || strings.HasSuffix(nameLower, "vm") {
		return "viewmodel"
	}
	if strings.HasSuffix(nameLower, "usecase") || strings.HasSuffix(nameLower, "interactor") {
		return "usecase"
	}
	if strings.HasSuffix(nameLower, "adapter") {
		return "adapter"
	}

	return ""
}
