package crawler

import (
	"context"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"

	"github.com/anurag/nexus/model"
)

type PythonParser struct {
	lang   *sitter.Language
	parser *sitter.Parser
}

func NewPythonParser() *PythonParser {
	lang := python.GetLanguage()
	p := sitter.NewParser()
	p.SetLanguage(lang)
	return &PythonParser{lang: lang, parser: p}
}

func (pp *PythonParser) Platform() model.Platform { return model.Python }

func (pp *PythonParser) Extensions() []string { return []string{".py"} }

func (pp *PythonParser) ParseFile(path string, content []byte) ([]model.Unit, error) {
	tree, err := pp.parser.ParseCtx(context.Background(), nil, content)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	var units []model.Unit
	root := tree.RootNode()
	pp.walkNode(root, content, path, &units)
	return units, nil
}

func (pp *PythonParser) walkNode(node *sitter.Node, src []byte, file string, units *[]model.Unit) {
	switch node.Type() {
	case "class_definition":
		unit := pp.extractClass(node, src, file)
		*units = append(*units, unit)
		return
	case "function_definition":
		unit := pp.extractFunction(node, src, file)
		*units = append(*units, unit)
		return
	case "decorated_definition":
		pp.extractDecorated(node, src, file, units)
		return
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		pp.walkNode(node.Child(i), src, file, units)
	}
}

func (pp *PythonParser) extractClass(node *sitter.Node, src []byte, file string) model.Unit {
	unit := model.Unit{
		Type: "class",
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
		if child.Type() == "block" {
			pp.extractClassMethods(child, src, file, &unit)
		}
	}
	return unit
}

func (pp *PythonParser) extractFunction(node *sitter.Node, src []byte, file string) model.Unit {
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
		if child.Type() == "parameters" {
			unit.Methods = append(unit.Methods, model.Method{
				Name:       unit.Name,
				Line:       unit.Line,
				Parameters: extractPythonParams(child, src),
			})
		}
	}
	pp.extractPythonSignals(node, src, &unit)
	return unit
}

func (pp *PythonParser) extractDecorated(node *sitter.Node, src []byte, file string, units *[]model.Unit) {
	var endpoint string
	var httpMethod string

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "decorator" {
			dec := child.Content(src)
			if strings.Contains(dec, "app.route") || strings.Contains(dec, "router.get") ||
				strings.Contains(dec, "router.post") || strings.Contains(dec, "router.put") ||
				strings.Contains(dec, "router.delete") {
				endpoint = extractStringArg(dec)
				if strings.Contains(dec, ".get") {
					httpMethod = "GET"
				} else if strings.Contains(dec, ".post") {
					httpMethod = "POST"
				} else if strings.Contains(dec, ".put") {
					httpMethod = "PUT"
				} else if strings.Contains(dec, ".delete") {
					httpMethod = "DELETE"
				} else {
					httpMethod = "GET"
				}
			}
		}
		if child.Type() == "function_definition" || child.Type() == "class_definition" {
			var innerUnits []model.Unit
			pp.walkNode(child, src, file, &innerUnits)
			for idx := range innerUnits {
				if endpoint != "" {
					innerUnits[idx].Endpoints = append(innerUnits[idx].Endpoints, httpMethod+" "+endpoint)
					innerUnits[idx].Stereotype = "endpoint"
				}
			}
			*units = append(*units, innerUnits...)
		}
	}
}

func (pp *PythonParser) extractClassMethods(block *sitter.Node, src []byte, file string, unit *model.Unit) {
	for i := 0; i < int(block.ChildCount()); i++ {
		child := block.Child(i)
		if child.Type() == "function_definition" {
			m := model.Method{
				Line: int(child.StartPoint().Row) + 1,
			}
			for j := 0; j < int(child.ChildCount()); j++ {
				mc := child.Child(j)
				if mc.Type() == "identifier" && m.Name == "" {
					m.Name = mc.Content(src)
				}
				if mc.Type() == "parameters" {
					m.Parameters = extractPythonParams(mc, src)
				}
			}
			unit.Methods = append(unit.Methods, m)
		}
		if child.Type() == "decorated_definition" {
			for j := 0; j < int(child.ChildCount()); j++ {
				mc := child.Child(j)
				if mc.Type() == "function_definition" {
					m := model.Method{
						Line: int(mc.StartPoint().Row) + 1,
					}
					for k := 0; k < int(mc.ChildCount()); k++ {
						fc := mc.Child(k)
						if fc.Type() == "identifier" && m.Name == "" {
							m.Name = fc.Content(src)
						}
					}
					unit.Methods = append(unit.Methods, m)
				}
			}
		}
	}
}

func (pp *PythonParser) extractPythonSignals(node *sitter.Node, src []byte, unit *model.Unit) {
	body := node.Content(src)
	lines := strings.Split(body, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "import ") || strings.HasPrefix(trimmed, "from ") {
			unit.Imports = append(unit.Imports, trimmed)
		}
		if strings.Contains(trimmed, "requests.get") || strings.Contains(trimmed, "requests.post") ||
			strings.Contains(trimmed, "httpx.") {
			url := extractStringArg(trimmed)
			if url != "" {
				unit.ApiCalls = append(unit.ApiCalls, url)
			}
		}
	}
}

func extractPythonParams(node *sitter.Node, src []byte) []string {
	var params []string
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" || child.Type() == "typed_parameter" || child.Type() == "default_parameter" {
			p := child.Content(src)
			if p != "self" && p != "cls" {
				params = append(params, p)
			}
		}
	}
	return params
}
