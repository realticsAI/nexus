package crawler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type ServiceSignals struct {
	Config    map[string]string
	BuildDeps []string
}

func CollectSignals(repoPath string) ServiceSignals {
	signals := ServiceSignals{
		Config: make(map[string]string),
	}
	collectAppYml(repoPath, &signals)
	collectPomDeps(repoPath, &signals)
	collectPackageJSON(repoPath, &signals)
	collectEnvFile(repoPath, &signals)
	return signals
}

func collectAppYml(repoPath string, s *ServiceSignals) {
	candidates := []string{
		"src/main/resources/application.yml",
		"src/main/resources/application.yaml",
		"src/main/resources/application.properties",
		"config/application.yml",
	}

	entries, err := os.ReadDir(repoPath)
	if err == nil {
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || e.Name() == "target" || e.Name() == "node_modules" || e.Name() == "build" {
				continue
			}
			for _, rel := range candidates {
				subPath := filepath.Join(repoPath, e.Name(), rel)
				if data, err := os.ReadFile(subPath); err == nil {
					if strings.HasSuffix(rel, ".properties") {
						parseProperties(string(data), s)
					} else {
						parseYAML(data, s)
					}
				}
			}
		}
	}

	for _, rel := range candidates {
		path := filepath.Join(repoPath, rel)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if strings.HasSuffix(rel, ".properties") {
			parseProperties(string(data), s)
		} else {
			parseYAML(data, s)
		}
		return
	}
}

func parseYAML(data []byte, s *ServiceSignals) {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return
	}
	flattenMap("", raw, s.Config)
}

func flattenMap(prefix string, m map[string]any, out map[string]string) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch val := v.(type) {
		case map[string]any:
			flattenMap(key, val, out)
		case string:
			out[key] = val
		case float64, int, bool:
			out[key] = strings.TrimRight(strings.TrimRight(
				strings.Replace(
					strings.Replace(
						strings.Replace(
							jsonString(val), "\"", "", -1),
						"true", "true", 1),
					"false", "false", 1),
				"0"), ".")
		}
	}
}

func jsonString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func parseProperties(content string, s *ServiceSignals) {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		s.Config[line[:idx]] = strings.TrimSpace(line[idx+1:])
	}
}

func collectPomDeps(repoPath string, s *ServiceSignals) {
	data, err := os.ReadFile(filepath.Join(repoPath, "pom.xml"))
	if err != nil {
		return
	}
	content := string(data)
	idx := 0
	for {
		start := strings.Index(content[idx:], "<artifactId>")
		if start < 0 {
			break
		}
		start += idx + len("<artifactId>")
		end := strings.Index(content[start:], "</artifactId>")
		if end < 0 {
			break
		}
		artifactId := content[start : start+end]
		if artifactId != "" && !strings.Contains(artifactId, "${") {
			s.BuildDeps = append(s.BuildDeps, artifactId)
		}
		idx = start + end
	}
}

func collectPackageJSON(repoPath string, s *ServiceSignals) {
	data, err := os.ReadFile(filepath.Join(repoPath, "package.json"))
	if err != nil {
		return
	}
	var pkg struct {
		Name            string            `json:"name"`
		Version         string            `json:"version"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
		Scripts         map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return
	}

	if pkg.Name != "" {
		s.Config["package.name"] = pkg.Name
	}
	if pkg.Version != "" {
		s.Config["package.version"] = pkg.Version
	}

	for dep := range pkg.Dependencies {
		s.BuildDeps = append(s.BuildDeps, dep)
	}

	frameworkKeys := []string{"react", "react-native", "next", "vue", "@angular/core", "svelte"}
	all := make(map[string]string, len(pkg.Dependencies)+len(pkg.DevDependencies))
	for k, v := range pkg.Dependencies {
		all[k] = v
	}
	for k, v := range pkg.DevDependencies {
		all[k] = v
	}
	for _, fw := range frameworkKeys {
		if ver, ok := all[fw]; ok {
			s.Config["framework."+fw] = ver
		}
	}
	if ver, ok := all["typescript"]; ok {
		s.Config["typescript.version"] = ver
	}

	if start, ok := pkg.Scripts["start"]; ok {
		if port := extractPortFromScript(start); port != "" {
			s.Config["package.port"] = port
		}
	}
	if dev, ok := pkg.Scripts["dev"]; ok {
		if port := extractPortFromScript(dev); port != "" {
			if _, exists := s.Config["package.port"]; !exists {
				s.Config["package.port"] = port
			}
		}
	}
}

var portFlagRe = regexp.MustCompile(`(?:--port|PORT=|-p)\s*(\d{2,5})`)

func extractPortFromScript(script string) string {
	if m := portFlagRe.FindStringSubmatch(script); len(m) > 1 {
		return m[1]
	}
	return ""
}

func collectEnvFile(repoPath string, s *ServiceSignals) {
	data, err := os.ReadFile(filepath.Join(repoPath, ".env"))
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx > 0 {
			s.Config["env."+line[:idx]] = line[idx+1:]
		}
	}
}
