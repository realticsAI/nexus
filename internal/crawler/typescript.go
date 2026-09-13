package crawler

import (
	"context"
	"regexp"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/typescript/typescript"

	"github.com/anurag/nexus/model"
)

type TypeScriptParser struct {
	lang   *sitter.Language
	parser *sitter.Parser
}

func NewTypeScriptParser() *TypeScriptParser {
	lang := typescript.GetLanguage()
	p := sitter.NewParser()
	p.SetLanguage(lang)
	return &TypeScriptParser{lang: lang, parser: p}
}

func (tp *TypeScriptParser) Platform() model.Platform { return model.TypeScript }

func (tp *TypeScriptParser) Extensions() []string { return []string{".ts", ".tsx", ".js", ".jsx"} }

func (tp *TypeScriptParser) ParseFile(path string, content []byte) ([]model.Unit, error) {
	tree, err := tp.parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	var units []model.Unit
	root := tree.RootNode()
	tp.walkNode(root, content, path, &units)

	tp.extractSignals(content, path, &units)
	return units, nil
}

func (tp *TypeScriptParser) walkNode(node *sitter.Node, src []byte, file string, units *[]model.Unit) {
	switch node.Type() {
	case "class_declaration":
		unit := tp.extractClass(node, src, file)
		*units = append(*units, unit)
		return
	case "function_declaration":
		unit := tp.extractFunction(node, src, file)
		if isScreenFunction(unit.Name, file) {
			unit.Stereotype = "screen"
			unit.Type = "component"
		}
		*units = append(*units, unit)
		return
	case "lexical_declaration":
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "variable_declarator" {
				content := child.Content(src)

				// Navigation config detection
				if isNavigatorDecl(content) {
					unit := tp.extractNavConfig(child, src, file, node, content)
					*units = append(*units, unit)
					return
				}

				// Redux slice detection
				if strings.Contains(content, "createSlice(") || strings.Contains(content, "createReducer(") {
					unit := tp.extractReduxSlice(child, src, file, node, content)
					*units = append(*units, unit)
					return
				}

				// Zustand store detection
				if isZustandStore(content) {
					unit := tp.extractZustandStore(child, src, file, node)
					*units = append(*units, unit)
					return
				}

				// Context provider detection
				if strings.Contains(content, "createContext(") {
					unit := tp.extractContextProvider(child, src, file, node)
					*units = append(*units, unit)
					return
				}

				// API hook detection (useXxxApi, useXxxService patterns)
				if isApiHook(child, src) {
					unit := tp.extractApiHook(child, src, file, node)
					*units = append(*units, unit)
					return
				}

				// React component detection (existing)
				if isComponentLike(child, src) {
					unit := tp.extractComponentDecl(child, src, file, node)
					if isScreenFunction(unit.Name, file) {
						unit.Stereotype = "screen"
					}
					*units = append(*units, unit)
					return
				}
			}
		}
	case "export_statement":
		for i := 0; i < int(node.ChildCount()); i++ {
			tp.walkNode(node.Child(i), src, file, units)
		}
		return
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		tp.walkNode(node.Child(i), src, file, units)
	}
}

func (tp *TypeScriptParser) extractClass(node *sitter.Node, src []byte, file string) model.Unit {
	unit := model.Unit{
		Type: "class",
		File: file,
		Line: int(node.StartPoint().Row) + 1,
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "type_identifier", "identifier":
			if unit.Name == "" {
				unit.Name = child.Content(src)
			}
		case "class_body":
			tp.extractClassFields(child, src, &unit)
		case "decorator":
			ann := child.Content(src)
			unit.Annotations = append(unit.Annotations, ann)
		}
	}
	unit.Stereotype = detectTSClassStereotype(unit)
	return unit
}

func (tp *TypeScriptParser) extractClassFields(bodyNode *sitter.Node, src []byte, unit *model.Unit) {
	for i := 0; i < int(bodyNode.ChildCount()); i++ {
		child := bodyNode.Child(i)

		if child.Type() == "method_definition" {
			name := ""
			for j := 0; j < int(child.ChildCount()); j++ {
				mc := child.Child(j)
				if mc.Type() == "property_identifier" {
					name = mc.Content(src)
				}
				if mc.Type() == "formal_parameters" && name == "constructor" {
					tp.extractConstructorInjections(mc, src, unit)
				}
			}
		}

		if child.Type() == "public_field_definition" || child.Type() == "property_declaration" {
			fieldText := child.Content(src)
			if strings.Contains(fieldText, "@Inject") || strings.Contains(fieldText, "@inject") {
				typeName := extractTSTypeAnnotation(fieldText)
				if typeName != "" && !isTSStdType(typeName) {
					unit.Dependencies = append(unit.Dependencies, typeName)
				}
			}
		}
	}
}

func (tp *TypeScriptParser) extractConstructorInjections(paramsNode *sitter.Node, src []byte, unit *model.Unit) {
	content := paramsNode.Content(src)
	for _, param := range strings.Split(content, ",") {
		param = strings.TrimSpace(param)
		if !strings.Contains(param, ":") {
			continue
		}
		parts := strings.SplitN(param, ":", 2)
		if len(parts) < 2 {
			continue
		}
		typeName := strings.TrimSpace(parts[1])
		typeName = strings.TrimRight(typeName, " )")
		if idx := strings.Index(typeName, "<"); idx >= 0 {
			typeName = typeName[:idx]
		}
		if typeName != "" && !isTSStdType(typeName) {
			unit.Dependencies = append(unit.Dependencies, typeName)
		}
	}
}

func extractTSTypeAnnotation(field string) string {
	if idx := strings.LastIndex(field, ":"); idx >= 0 {
		typeName := strings.TrimSpace(field[idx+1:])
		typeName = strings.TrimRight(typeName, ";")
		typeName = strings.TrimSpace(typeName)
		if i := strings.Index(typeName, "<"); i >= 0 {
			typeName = typeName[:i]
		}
		return typeName
	}
	return ""
}

func isTSStdType(name string) bool {
	switch name {
	case "string", "number", "boolean", "void", "any", "unknown", "never",
		"Date", "Promise", "Array", "Record", "Map", "Set", "Error",
		"Request", "Response", "Buffer", "EventEmitter":
		return true
	}
	return false
}

func detectTSClassStereotype(unit model.Unit) string {
	for _, ann := range unit.Annotations {
		switch {
		case strings.Contains(ann, "@Controller") || strings.Contains(ann, "@RestController"):
			return "controller"
		case strings.Contains(ann, "@Injectable"):
			return "service"
		case strings.Contains(ann, "@Module"):
			return "module"
		case strings.Contains(ann, "@Entity"):
			return "entity"
		}
	}
	nameLower := strings.ToLower(unit.Name)
	if strings.HasSuffix(nameLower, "service") {
		return "service"
	}
	if strings.HasSuffix(nameLower, "controller") {
		return "controller"
	}
	if strings.HasSuffix(nameLower, "repository") {
		return "repository"
	}
	return ""
}

func (tp *TypeScriptParser) extractFunction(node *sitter.Node, src []byte, file string) model.Unit {
	unit := model.Unit{
		Type: "function",
		File: file,
		Line: int(node.StartPoint().Row) + 1,
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" {
			if unit.Name == "" {
				unit.Name = child.Content(src)
			}
		}
	}
	return unit
}

func (tp *TypeScriptParser) extractComponentDecl(node *sitter.Node, src []byte, file string, parent *sitter.Node) model.Unit {
	unit := model.Unit{
		Type:       "component",
		File:       file,
		Line:       int(parent.StartPoint().Row) + 1,
		Stereotype: "react_component",
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" {
			if unit.Name == "" {
				unit.Name = child.Content(src)
			}
		}
	}
	return unit
}

// --- Navigation detection ---

var navigatorCreators = []string{
	"createStackNavigator",
	"createNativeStackNavigator",
	"createBottomTabNavigator",
	"createMaterialTopTabNavigator",
	"createDrawerNavigator",
	"createMaterialBottomTabNavigator",
}

func isNavigatorDecl(content string) bool {
	for _, creator := range navigatorCreators {
		if strings.Contains(content, creator+"(") {
			return true
		}
	}
	return false
}

func (tp *TypeScriptParser) extractNavConfig(node *sitter.Node, src []byte, file string, parent *sitter.Node, content string) model.Unit {
	unit := model.Unit{
		Type:       "component",
		File:       file,
		Line:       int(parent.StartPoint().Row) + 1,
		Stereotype: "navigation_config",
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" {
			if unit.Name == "" {
				unit.Name = child.Content(src)
			}
		}
	}

	// Determine navigator type from the creator function
	for _, creator := range navigatorCreators {
		if strings.Contains(content, creator) {
			unit.Annotations = append(unit.Annotations, creator)
			break
		}
	}

	return unit
}

var screenNameRe = regexp.MustCompile(`\.Screen\s+name=["']([^"']+)["']`)
var navigateRe = regexp.MustCompile(`navigation\.navigate\(\s*["']([^"']+)["']`)
var pushRe = regexp.MustCompile(`navigation\.push\(\s*["']([^"']+)["']`)
var resetRe = regexp.MustCompile(`navigation\.reset\(.*?name:\s*["']([^"']+)["']`)

// --- Redux/state detection ---

var sliceNameRe = regexp.MustCompile(`createSlice\(\s*\{\s*name:\s*["']([^"']+)["']`)

func (tp *TypeScriptParser) extractReduxSlice(node *sitter.Node, src []byte, file string, parent *sitter.Node, content string) model.Unit {
	unit := model.Unit{
		Type:       "store",
		File:       file,
		Line:       int(parent.StartPoint().Row) + 1,
		Stereotype: "redux_slice",
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" {
			if unit.Name == "" {
				unit.Name = child.Content(src)
			}
		}
	}

	if m := sliceNameRe.FindStringSubmatch(content); len(m) > 1 {
		unit.Annotations = append(unit.Annotations, "slice:"+m[1])
	}

	// Extract reducer action names from the content
	reducerRe := regexp.MustCompile(`(\w+)\s*:\s*\(state`)
	for _, m := range reducerRe.FindAllStringSubmatch(content, -1) {
		if len(m) > 1 {
			unit.Methods = append(unit.Methods, model.Method{Name: m[1]})
		}
	}

	return unit
}

func isZustandStore(content string) bool {
	return (strings.Contains(content, "create(") || strings.Contains(content, "create<")) &&
		strings.Contains(content, "set") &&
		(strings.Contains(content, "=>") || strings.Contains(content, "function"))
}

func (tp *TypeScriptParser) extractZustandStore(node *sitter.Node, src []byte, file string, parent *sitter.Node) model.Unit {
	unit := model.Unit{
		Type:       "store",
		File:       file,
		Line:       int(parent.StartPoint().Row) + 1,
		Stereotype: "zustand_store",
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" {
			if unit.Name == "" {
				unit.Name = child.Content(src)
			}
		}
	}
	return unit
}

// --- Context provider detection ---

func (tp *TypeScriptParser) extractContextProvider(node *sitter.Node, src []byte, file string, parent *sitter.Node) model.Unit {
	unit := model.Unit{
		Type:       "context",
		File:       file,
		Line:       int(parent.StartPoint().Row) + 1,
		Stereotype: "context_provider",
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" {
			if unit.Name == "" {
				unit.Name = child.Content(src)
			}
		}
	}
	return unit
}

// --- API hook detection ---

var apiHookRe = regexp.MustCompile(`^use\w+(Api|Service|Client|Fetch|Query)$`)

func isApiHook(node *sitter.Node, src []byte) bool {
	content := node.Content(src)
	if !strings.Contains(content, "=>") && !strings.Contains(content, "function") {
		return false
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" {
			name := child.Content(src)
			if apiHookRe.MatchString(name) {
				return true
			}
		}
	}
	return false
}

func (tp *TypeScriptParser) extractApiHook(node *sitter.Node, src []byte, file string, parent *sitter.Node) model.Unit {
	unit := model.Unit{
		Type:       "function",
		File:       file,
		Line:       int(parent.StartPoint().Row) + 1,
		Stereotype: "api_hook",
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" {
			if unit.Name == "" {
				unit.Name = child.Content(src)
			}
		}
	}
	return unit
}

// --- Screen heuristic ---

func isScreenFunction(name, file string) bool {
	nameLower := strings.ToLower(name)
	fileLower := strings.ToLower(file)
	return strings.HasSuffix(nameLower, "screen") ||
		strings.HasSuffix(nameLower, "page") ||
		strings.HasSuffix(nameLower, "view") ||
		strings.Contains(fileLower, "/screens/") ||
		strings.Contains(fileLower, "/pages/") ||
		strings.Contains(fileLower, "/views/")
}

func isComponentLike(node *sitter.Node, src []byte) bool {
	content := node.Content(src)
	return strings.Contains(content, "=>") &&
		(strings.Contains(content, "jsx") || strings.Contains(content, "return") ||
			strings.Contains(content, "<") || strings.Contains(content, "React"))
}

// --- Test file detection ---

func isTestFile(file string) bool {
	lower := strings.ToLower(file)
	base := lower
	if idx := strings.LastIndex(lower, "/"); idx >= 0 {
		base = lower[idx+1:]
	}
	if strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
		return true
	}
	if strings.HasPrefix(base, "test_") || base == "test.ts" || base == "test.tsx" || base == "test.js" || base == "test.jsx" {
		return true
	}
	return strings.Contains(lower, "/__tests__/") ||
		strings.Contains(lower, "/__mocks__/") ||
		strings.Contains(lower, "/test/") ||
		strings.Contains(lower, "/tests/") ||
		strings.HasPrefix(lower, "test/") ||
		strings.HasPrefix(lower, "tests/")
}

// --- Signal extraction (line-based) ---

func (tp *TypeScriptParser) extractSignals(content []byte, file string, units *[]model.Unit) {
	src := string(content)
	lines := strings.Split(src, "\n")
	isTest := isTestFile(file)

	var screenEndpoints []string
	var apiCalls []string
	var navDeps []string
	var annotations []string
	var configRefs []string

	if isTest {
		for i := range *units {
			(*units)[i].IsTest = true
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Imports
		if strings.HasPrefix(trimmed, "import ") {
			if len(*units) > 0 {
				(*units)[0].Imports = append((*units)[0].Imports, trimmed)
			}
		}

		// --- Phase 1: Navigation signals ---

		// Screen registrations: <Stack.Screen name="Commerce" .../>
		for _, m := range screenNameRe.FindAllStringSubmatch(trimmed, -1) {
			if len(m) > 1 {
				screenEndpoints = append(screenEndpoints, "SCREEN /"+m[1])
			}
		}

		// navigation.navigate('ScreenName') calls
		for _, m := range navigateRe.FindAllStringSubmatch(trimmed, -1) {
			if len(m) > 1 {
				navDeps = append(navDeps, m[1])
			}
		}
		for _, m := range pushRe.FindAllStringSubmatch(trimmed, -1) {
			if len(m) > 1 {
				navDeps = append(navDeps, m[1])
			}
		}
		for _, m := range resetRe.FindAllStringSubmatch(trimmed, -1) {
			if len(m) > 1 {
				navDeps = append(navDeps, m[1])
			}
		}

		// useNavigation/useRoute hooks mark component as screen-aware
		if strings.Contains(trimmed, "useNavigation") || strings.Contains(trimmed, "useRoute") {
			annotations = appendUniq(annotations, "navigation-aware")
		}

		// --- Phase 2: State management signals ---

		if strings.Contains(trimmed, "useSelector(") || strings.Contains(trimmed, "useAppSelector(") {
			annotations = appendUniq(annotations, "redux-consumer")
		}
		if strings.Contains(trimmed, "useDispatch(") || strings.Contains(trimmed, "useAppDispatch(") {
			annotations = appendUniq(annotations, "redux-dispatcher")
		}
		if strings.Contains(trimmed, "useContext(") {
			annotations = appendUniq(annotations, "context-consumer")
		}

		// --- Phase 3: Native bridge & API layer signals ---
		// Skip API call extraction for test files — test fixtures create
		// false FRONTEND_API_CALL edges in the graph.

		// NativeModules.ModuleName
		if !isTest && strings.Contains(trimmed, "NativeModules.") {
			moduleName := extractNativeModuleName(trimmed)
			if moduleName != "" {
				apiCalls = append(apiCalls, "NATIVE://"+moduleName)
			}
		}

		// Platform detection
		if strings.Contains(trimmed, "Platform.OS") || strings.Contains(trimmed, "Platform.select(") {
			annotations = appendUniq(annotations, "platform-specific")
		}

		// fetch / axios calls — collapse template literals before extracting
		if !isTest && (strings.Contains(trimmed, "fetch(") || strings.Contains(trimmed, "axios.")) {
			url := extractStringArg(trimmed)
			if url != "" {
				collapsed := collapseTemplateLiteral(url)
				if strings.HasPrefix(collapsed, "/") || strings.HasPrefix(collapsed, "http") {
					apiCalls = append(apiCalls, collapsed)
				}
			}
		}

		// axios.create({ baseURL: "..." })
		if !isTest {
			if m := axiosCreateRe.FindStringSubmatch(trimmed); len(m) > 1 {
				apiCalls = append(apiCalls, m[1])
			}
		}

		// Base URL constants: const BASE_URL = "/api/v1"
		if !isTest {
			if m := baseUrlConstRe.FindStringSubmatch(trimmed); len(m) > 1 {
				val := m[1]
				if strings.HasPrefix(val, "/") || strings.HasPrefix(val, "http") {
					apiCalls = append(apiCalls, val)
				}
			}
		}

		// Path literals: '/api/orders/{id}/items'
		if !isTest {
			for _, m := range pathLiteralRe.FindAllStringSubmatch(trimmed, -1) {
				if len(m) > 1 {
					collapsed := collapseTemplateLiteral(m[1])
					apiCalls = append(apiCalls, collapsed)
				}
			}
		}

		// Apollo/urql URI config: uri: "https://graph.example.com/graphql"
		if !isTest {
			if m := apolloUriRe.FindStringSubmatch(trimmed); len(m) > 1 {
				val := m[1]
				if strings.HasPrefix(val, "/") || strings.HasPrefix(val, "http") {
					apiCalls = append(apiCalls, val)
				}
			}
		}

		// useQuery / useMutation with query keys or URLs
		if !isTest && (strings.Contains(trimmed, "useQuery") || strings.Contains(trimmed, "useMutation")) {
			url := extractStringArg(trimmed)
			if url != "" {
				apiCalls = append(apiCalls, url)
			}
		}

		// GraphQL operations: gql`query GetProducts { ... }`
		if !isTest && (strings.Contains(trimmed, "gql`") || strings.Contains(trimmed, "gql(")) {
			opName := extractGqlOperationName(trimmed)
			if opName != "" {
				apiCalls = append(apiCalls, "GQL://"+opName)
			}
		}

		// --- Phase 4: Environment variable references ---
		for _, m := range processEnvRe.FindAllStringSubmatch(trimmed, -1) {
			if len(m) > 1 && !envVarNoise[m[1]] {
				configRefs = appendUniq(configRefs, "env:"+m[1])
			}
		}
		for _, m := range importMetaEnvRe.FindAllStringSubmatch(trimmed, -1) {
			if len(m) > 1 && !envVarNoise[m[1]] {
				configRefs = appendUniq(configRefs, "env:"+m[1])
			}
		}
	}

	// Apply collected signals to the first unit (file-level)
	if len(*units) > 0 {
		first := &(*units)[0]
		first.ApiCalls = append(first.ApiCalls, apiCalls...)
		first.Endpoints = append(first.Endpoints, screenEndpoints...)
		first.Dependencies = append(first.Dependencies, navDeps...)
		first.Annotations = append(first.Annotations, annotations...)
		first.ConfigRefs = append(first.ConfigRefs, configRefs...)
	}

	// Also apply screen endpoints to any navigation_config unit
	for i := range *units {
		if (*units)[i].Stereotype == "navigation_config" {
			(*units)[i].Endpoints = append((*units)[i].Endpoints, screenEndpoints...)
		}
	}
}

// --- Helpers ---

// --- Deep API URL extraction patterns ---

var axiosCreateRe = regexp.MustCompile(`axios\.create\([^)]*baseURL\s*:\s*['"\x60]([^'"\x60]+)['"\x60]`)
var baseUrlConstRe = regexp.MustCompile(`(?:baseURL|BASE_URL|apiUrl|API_URL|API_BASE|baseUrl)\s*[=:]\s*['"\x60]([^'"\x60]+)['"\x60]`)
var pathLiteralRe = regexp.MustCompile(`['"\x60](/[a-zA-Z][a-zA-Z0-9_-]*(?:/[a-zA-Z0-9{}$._:\-]+)+)['"\x60]`)
var apolloUriRe = regexp.MustCompile(`(?:uri|url)\s*:\s*['"\x60]([^'"\x60]+)['"\x60]`)
var templateInterpRe = regexp.MustCompile(`\$\{[^}]*}`)

var processEnvRe = regexp.MustCompile(`process\.env\.([A-Z_][A-Z0-9_]*)`)
var importMetaEnvRe = regexp.MustCompile(`import\.meta\.env\.([A-Z_][A-Z0-9_]*)`)

var envVarNoise = map[string]bool{
	"NODE_ENV": true, "PORT": true, "DEBUG": true, "CI": true,
	"HOME": true, "PATH": true, "PWD": true, "SHELL": true,
	"TERM": true, "USER": true, "LANG": true, "TZ": true,
	"HOSTNAME": true, "npm_lifecycle_event": true,
}

var nativeModuleRe = regexp.MustCompile(`NativeModules\.(\w+)`)

func extractNativeModuleName(line string) string {
	m := nativeModuleRe.FindStringSubmatch(line)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

var gqlOpRe = regexp.MustCompile(`(?:query|mutation|subscription)\s+(\w+)`)

func extractGqlOperationName(line string) string {
	m := gqlOpRe.FindStringSubmatch(line)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

func appendUniq(slice []string, val string) []string {
	for _, s := range slice {
		if s == val {
			return slice
		}
	}
	return append(slice, val)
}

// collapseTemplateLiteral replaces ${...} interpolations with {id} so
// template-literal URLs become matchable against controller paths.
// e.g. `${API_BASE}/cart/${id}/items` → `/cart/{id}/items`
func collapseTemplateLiteral(s string) string {
	collapsed := templateInterpRe.ReplaceAllString(s, "{id}")
	// After collapsing, strip leading {id} segments (the base URL variable)
	// so `{id}/cart/{id}/items` becomes `/cart/{id}/items`.
	for strings.HasPrefix(collapsed, "{id}") {
		collapsed = strings.TrimPrefix(collapsed, "{id}")
	}
	return collapsed
}

// extractApiUrlFromLine extracts a URL from a line, collapsing template
// literals. Returns empty string if no usable path is found.
func extractApiUrlFromLine(line string) string {
	raw := extractStringArg(line)
	if raw == "" {
		return ""
	}
	cleaned := collapseTemplateLiteral(raw)
	cleaned = strings.TrimSpace(cleaned)
	if !strings.Contains(cleaned, "/") {
		return ""
	}
	if cleaned == "/" || cleaned == "/{id}" {
		return ""
	}
	return cleaned
}
