package crawler

import (
	"regexp"
	"strings"

	"github.com/anurag/nexus/model"
)

type SwiftParser struct{}

func NewSwiftParser() *SwiftParser { return &SwiftParser{} }

func (sp *SwiftParser) Platform() model.Platform { return model.Swift }

func (sp *SwiftParser) Extensions() []string { return []string{".swift"} }

var (
	swiftClassRe    = regexp.MustCompile(`^\s*(?:(?:public|internal|private|fileprivate|open|final)\s+)*class\s+(\w+)(?:\s*:\s*(.+))?\s*\{`)
	swiftStructRe   = regexp.MustCompile(`^\s*(?:(?:public|internal|private|fileprivate)\s+)*struct\s+(\w+)(?:\s*:\s*(.+))?\s*\{`)
	swiftEnumRe     = regexp.MustCompile(`^\s*(?:(?:public|internal|private|fileprivate)\s+)*enum\s+(\w+)(?:\s*:\s*(.+))?\s*\{`)
	swiftProtocolRe = regexp.MustCompile(`^\s*(?:(?:public|internal|private|fileprivate)\s+)*protocol\s+(\w+)(?:\s*:\s*(.+))?\s*\{`)
	swiftFuncRe     = regexp.MustCompile(`^\s*(?:(?:@\w+(?:\(.*?\))?\s+)*)(?:(?:public|internal|private|fileprivate|open|static|class|override|mutating)\s+)*func\s+(\w+)\s*\(([^)]*)\)(?:\s*(?:throws\s+)?->\s*(.+))?\s*\{?`)
	swiftImportRe   = regexp.MustCompile(`^\s*import\s+(\w+)`)
	swiftWrapperRe  = regexp.MustCompile(`@(Published|State|Binding|ObservedObject|EnvironmentObject|StateObject|AppStorage|SceneStorage|FetchRequest|Environment)`)
	swiftIBActionRe = regexp.MustCompile(`@IBAction\s+func\s+(\w+)`)
	swiftIBOutletRe = regexp.MustCompile(`@IBOutlet`)
	swiftURLRe      = regexp.MustCompile(`(?:URL\(string:\s*"([^"]+)"|URLSession\.shared\.\w+|AF\.request\(\s*"([^"]+)"|Moya|\.request\(\s*"([^"]+)")`)
	swiftConfigRe   = regexp.MustCompile(`(?:Bundle\.main\.infoDictionary|UserDefaults\.standard|ProcessInfo\.processInfo\.environment|\.plist)`)
	swiftPropRe     = regexp.MustCompile(`^\s*(?:(?:public|internal|private|fileprivate)\s+)?(?:let|var)\s+\w+\s*:\s*(\w+)`)
	swiftInjectRe   = regexp.MustCompile(`@(?:Inject|Injected|LazyInjected)\s+(?:var|let)\s+\w+\s*:\s*(\w+)`)
)

func (sp *SwiftParser) ParseFile(path string, content []byte) ([]model.Unit, error) {
	lines := strings.Split(string(content), "\n")
	var units []model.Unit
	var currentUnit *model.Unit
	braceDepth := 0
	inType := false

	for lineNum, line := range lines {
		trimmed := strings.TrimSpace(line)

		if m := swiftImportRe.FindStringSubmatch(trimmed); m != nil {
			if currentUnit != nil {
				currentUnit.Imports = append(currentUnit.Imports, m[1])
			} else if len(units) > 0 {
				units[0].Imports = append(units[0].Imports, m[1])
			}
		}

		if !inType {
			if u := sp.matchTypeDecl(trimmed, path, lineNum+1); u != nil {
				if currentUnit != nil {
					units = append(units, *currentUnit)
				}
				currentUnit = u
				inType = true
				braceDepth = 1
				continue
			}
		}

		if inType && currentUnit != nil {
			braceDepth += strings.Count(line, "{") - strings.Count(line, "}")

			if m := swiftFuncRe.FindStringSubmatch(trimmed); m != nil {
				method := model.Method{
					Name:       m[1],
					Line:       lineNum + 1,
					ReturnType: strings.TrimSpace(m[3]),
				}
				if m[2] != "" {
					method.Parameters = parseSwiftParams(m[2])
				}
				if swiftIBActionRe.MatchString(trimmed) {
					method.Annotations = append(method.Annotations, "@IBAction")
				}
				currentUnit.Methods = append(currentUnit.Methods, method)
			}

			if m := swiftInjectRe.FindStringSubmatch(trimmed); m != nil {
				dep := m[1]
				if !isSwiftStdType(dep) {
					currentUnit.Dependencies = appendUniqueStr(currentUnit.Dependencies, dep)
				}
			} else if m := swiftPropRe.FindStringSubmatch(trimmed); m != nil {
				dep := m[1]
				if !isSwiftStdType(dep) && isPrivateOrLetDecl(trimmed) {
					currentUnit.Dependencies = appendUniqueStr(currentUnit.Dependencies, dep)
				}
			}

			if ms := swiftWrapperRe.FindAllStringSubmatch(trimmed, -1); ms != nil {
				for _, m := range ms {
					currentUnit.Annotations = appendUniqueStr(currentUnit.Annotations, "@"+m[1])
				}
			}

			if swiftIBOutletRe.MatchString(trimmed) {
				currentUnit.Annotations = appendUniqueStr(currentUnit.Annotations, "@IBOutlet")
			}

			sp.extractSignals(trimmed, currentUnit)

			if braceDepth <= 0 {
				units = append(units, *currentUnit)
				currentUnit = nil
				inType = false
			}
		}
	}

	if currentUnit != nil {
		units = append(units, *currentUnit)
	}

	for i := range units {
		if units[i].Stereotype == "" {
			units[i].Stereotype = sp.inferStereotype(&units[i])
		}
	}

	return units, nil
}

func (sp *SwiftParser) matchTypeDecl(line, file string, lineNum int) *model.Unit {
	if m := swiftClassRe.FindStringSubmatch(line); m != nil {
		return &model.Unit{
			Type:       "class",
			Name:       m[1],
			File:       file,
			Line:       lineNum,
			Stereotype: sp.stereotypeFromInheritance("class", m[1], m[2]),
		}
	}
	if m := swiftStructRe.FindStringSubmatch(line); m != nil {
		return &model.Unit{
			Type:       "struct",
			Name:       m[1],
			File:       file,
			Line:       lineNum,
			Stereotype: sp.stereotypeFromInheritance("struct", m[1], m[2]),
		}
	}
	if m := swiftEnumRe.FindStringSubmatch(line); m != nil {
		return &model.Unit{
			Type: "enum",
			Name: m[1],
			File: file,
			Line: lineNum,
		}
	}
	if m := swiftProtocolRe.FindStringSubmatch(line); m != nil {
		return &model.Unit{
			Type: "protocol",
			Name: m[1],
			File: file,
			Line: lineNum,
		}
	}
	return nil
}

func (sp *SwiftParser) stereotypeFromInheritance(typeKind, name, inheritance string) string {
	if inheritance == "" {
		return sp.stereotypeFromName(name)
	}

	parts := strings.Split(inheritance, ",")
	for _, part := range parts {
		p := strings.TrimSpace(part)
		switch {
		case p == "UIViewController" || p == "UITableViewController" ||
			p == "UICollectionViewController" || p == "UINavigationController" ||
			p == "UITabBarController" || p == "UIPageViewController" ||
			strings.HasSuffix(p, "ViewController"):
			return "viewcontroller"
		case p == "View" && typeKind == "struct":
			return "swiftui_view"
		case p == "ObservableObject":
			return "viewmodel"
		case p == "NSManagedObject" || p == "NSFetchRequestResult":
			return "entity"
		case p == "Codable" || p == "Decodable" || p == "Encodable":
			return "model"
		case p == "URLSessionDelegate" || p == "URLSessionDataDelegate":
			return "api_service"
		case p == "UIApplicationDelegate":
			return "app_delegate"
		}
	}

	return sp.stereotypeFromName(name)
}

func (sp *SwiftParser) stereotypeFromName(name string) string {
	nameLower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(nameLower, "viewcontroller"):
		return "viewcontroller"
	case strings.HasSuffix(nameLower, "viewmodel"):
		return "viewmodel"
	case strings.HasSuffix(nameLower, "service") || strings.HasSuffix(nameLower, "client") ||
		strings.HasSuffix(nameLower, "api"):
		return "api_service"
	case strings.HasSuffix(nameLower, "repository") || strings.HasSuffix(nameLower, "store") ||
		strings.HasSuffix(nameLower, "dao"):
		return "repository"
	case strings.HasSuffix(nameLower, "manager"):
		return "repository"
	case strings.HasSuffix(nameLower, "coordinator") || strings.HasSuffix(nameLower, "router"):
		return "coordinator"
	case strings.HasSuffix(nameLower, "view") && !strings.HasSuffix(nameLower, "review"):
		return "swiftui_view"
	case strings.HasSuffix(nameLower, "cell"):
		return "view_cell"
	case strings.HasSuffix(nameLower, "request") || strings.HasSuffix(nameLower, "response") ||
		strings.HasSuffix(nameLower, "dto") || strings.HasSuffix(nameLower, "model"):
		return "model"
	case strings.HasSuffix(nameLower, "config") || strings.HasSuffix(nameLower, "configuration"):
		return "configuration"
	}
	return ""
}

func (sp *SwiftParser) inferStereotype(unit *model.Unit) string {
	if s := sp.stereotypeFromName(unit.Name); s != "" {
		return s
	}
	for _, ann := range unit.Annotations {
		if ann == "@Published" || ann == "@StateObject" {
			return "viewmodel"
		}
		if ann == "@State" || ann == "@Binding" {
			return "swiftui_view"
		}
	}
	if len(unit.ApiCalls) > 0 {
		return "api_service"
	}
	return ""
}

func (sp *SwiftParser) extractSignals(line string, unit *model.Unit) {
	if ms := swiftURLRe.FindAllStringSubmatch(line, -1); ms != nil {
		for _, m := range ms {
			for _, url := range m[1:] {
				if url != "" {
					unit.ApiCalls = appendUniqueStr(unit.ApiCalls, url)
				}
			}
		}
	}

	if strings.Contains(line, "URLSession.shared") || strings.Contains(line, "AF.request") {
		url := extractStringArg(line)
		if url != "" && (strings.HasPrefix(url, "/") || strings.HasPrefix(url, "http")) {
			unit.ApiCalls = appendUniqueStr(unit.ApiCalls, url)
		}
	}

	if swiftConfigRe.MatchString(line) {
		key := extractStringArg(line)
		if key != "" {
			unit.ConfigRefs = appendUniqueStr(unit.ConfigRefs, key)
		}
	}

	if strings.Contains(line, "PassthroughSubject") || strings.Contains(line, "CurrentValueSubject") ||
		strings.Contains(line, ".publisher") {
		unit.Annotations = appendUniqueStr(unit.Annotations, "reactive:combine")
	}
}

func parseSwiftParams(raw string) []string {
	if raw == "" {
		return nil
	}
	var params []string
	depth := 0
	current := ""
	for _, ch := range raw {
		switch ch {
		case '(':
			depth++
			current += string(ch)
		case ')':
			depth--
			current += string(ch)
		case ',':
			if depth == 0 {
				p := strings.TrimSpace(current)
				if p != "" {
					params = append(params, p)
				}
				current = ""
			} else {
				current += string(ch)
			}
		default:
			current += string(ch)
		}
	}
	if p := strings.TrimSpace(current); p != "" {
		params = append(params, p)
	}
	return params
}

func isSwiftStdType(name string) bool {
	switch name {
	case "String", "Int", "Double", "Float", "Bool", "Data", "Date", "URL",
		"Array", "Dictionary", "Set", "Optional", "Result",
		"Error", "NSError", "Void", "Any", "AnyObject",
		"UIImage", "UIColor", "CGFloat", "CGRect", "CGSize",
		"NSAttributedString", "IndexPath", "Notification",
		"Published", "State", "Binding":
		return true
	}
	return false
}

func isPrivateOrLetDecl(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.Contains(trimmed, "private ") ||
		(strings.Contains(trimmed, "let ") && !strings.HasPrefix(trimmed, "guard"))
}

func appendUniqueStr(slice []string, val string) []string {
	for _, s := range slice {
		if s == val {
			return slice
		}
	}
	return append(slice, val)
}
