package agent

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ScanLibraryAPIs reads pom.xml/build.gradle, finds internal (com.example.platform.*) dependencies,
// locates their JARs in ~/.m2, and extracts class signatures with javap.
// Only includes classes actually imported by the repo's source code.
func ScanLibraryAPIs(repoPath string) []LibraryAPI {
	usedImports := collectRepoImports(repoPath)

	pomPath := filepath.Join(repoPath, "pom.xml")
	if _, err := os.Stat(pomPath); err == nil {
		return scanMavenDeps(pomPath, usedImports)
	}
	gradlePath := filepath.Join(repoPath, "build.gradle")
	if _, err := os.Stat(gradlePath); err == nil {
		return scanGradleDeps(gradlePath)
	}
	return nil
}

// collectRepoImports scans all .java files for import statements
// and returns the set of fully-qualified class names imported.
func collectRepoImports(repoPath string) map[string]bool {
	imports := make(map[string]bool)
	srcPath := filepath.Join(repoPath, "src", "main")
	filepath.Walk(srcPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".java") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for _, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "import ") {
				imp := strings.TrimSuffix(strings.TrimPrefix(trimmed, "import "), ";")
				imp = strings.TrimPrefix(imp, "static ")
				imports[imp] = true
				// Also mark the package as used (for wildcard-like resolution)
				if idx := strings.LastIndex(imp, "."); idx > 0 {
					imports[imp[:idx]+".*"] = true
				}
			}
		}
		return nil
	})
	return imports
}

type pomProject struct {
	Properties pomProperties `xml:"properties"`
	Dependencies struct {
		Dependency []pomDep `xml:"dependency"`
	} `xml:"dependencies"`
}

type pomProperties struct {
	Entries []pomProperty `xml:",any"`
}

type pomProperty struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
}

type pomDep struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Scope      string `xml:"scope"`
}

func scanMavenDeps(pomPath string, usedImports map[string]bool) []LibraryAPI {
	data, err := os.ReadFile(pomPath)
	if err != nil {
		return nil
	}

	var pom pomProject
	if err := xml.Unmarshal(data, &pom); err != nil {
		return nil
	}

	props := buildPropertyMap(pom.Properties)

	var apis []LibraryAPI
	for _, dep := range pom.Dependencies.Dependency {
		if dep.Scope == "test" {
			continue
		}
		if !isInternalGroup(dep.GroupID) {
			continue
		}
		version := resolveProperty(dep.Version, props)
		if version == "" {
			continue
		}
		api := extractJARAPI(dep.GroupID, dep.ArtifactID, version, usedImports)
		if api != nil && len(api.Classes) > 0 {
			apis = append(apis, *api)
		}
	}
	return apis
}

func buildPropertyMap(props pomProperties) map[string]string {
	m := make(map[string]string)
	for _, p := range props.Entries {
		m[p.XMLName.Local] = strings.TrimSpace(p.Value)
	}
	return m
}

func resolveProperty(value string, props map[string]string) string {
	if !strings.HasPrefix(value, "${") || !strings.HasSuffix(value, "}") {
		return value
	}
	key := value[2 : len(value)-1]
	if resolved, ok := props[key]; ok {
		return resolved
	}
	return ""
}

func scanGradleDeps(gradlePath string) []LibraryAPI {
	// TODO: parse build.gradle for implementation/api dependencies
	return nil
}

func isInternalGroup(groupID string) bool {
	return strings.HasPrefix(groupID, "com.example.platform.") ||
		strings.HasPrefix(groupID, "com.example.")
}

func extractJARAPI(groupID, artifactID, version string, usedImports map[string]bool) *LibraryAPI {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	groupPath := strings.ReplaceAll(groupID, ".", "/")
	jarPath := filepath.Join(home, ".m2", "repository", groupPath, artifactID, version,
		fmt.Sprintf("%s-%s.jar", artifactID, version))

	if _, err := os.Stat(jarPath); err != nil {
		return nil
	}

	classes := listJARClasses(jarPath)
	if len(classes) == 0 {
		return nil
	}

	api := &LibraryAPI{
		GroupID:    groupID,
		ArtifactID: artifactID,
		Version:    version,
	}

	// Filter: only extract classes actually imported by the repo,
	// plus their direct dependencies (types in their fields/methods)
	importedClasses := map[string]bool{}
	for _, className := range classes {
		if usedImports[className] {
			importedClasses[className] = true
		}
		// Check wildcard: if repo imports com.foo.bar.*, include com.foo.bar.Baz
		pkg := className
		if idx := strings.LastIndex(className, "."); idx > 0 {
			pkg = className[:idx] + ".*"
		}
		if usedImports[pkg] {
			importedClasses[className] = true
		}
	}

	// Pass 1: decompile directly imported classes, discover field-type dependencies
	decompiled := map[string]bool{}
	for _, className := range classes {
		if !importedClasses[className] {
			continue
		}
		lc := decompileClass(jarPath, className)
		if lc != nil {
			api.Classes = append(api.Classes, *lc)
			decompiled[className] = true
			for _, f := range lc.Fields {
				for _, other := range classes {
					parts := strings.Split(other, ".")
					shortName := parts[len(parts)-1]
					if strings.Contains(f, shortName) && !importedClasses[other] {
						importedClasses[other] = true
					}
				}
			}
		}
	}

	// Pass 2: decompile discovered field-type classes
	for _, className := range classes {
		if !importedClasses[className] || decompiled[className] {
			continue
		}
		lc := decompileClass(jarPath, className)
		if lc != nil {
			api.Classes = append(api.Classes, *lc)
		}
		if len(api.Classes) >= 20 {
			break
		}
	}

	return api
}

func listJARClasses(jarPath string) []string {
	out, err := exec.Command("jar", "tf", jarPath).Output()
	if err != nil {
		return nil
	}

	var classes []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, ".class") && !strings.Contains(line, "$") {
			className := strings.TrimSuffix(line, ".class")
			className = strings.ReplaceAll(className, "/", ".")
			classes = append(classes, className)
		}
	}
	return classes
}

func isRelevantLibraryClass(className string) bool {
	lower := strings.ToLower(className)
	parts := strings.Split(className, ".")
	shortName := parts[len(parts)-1]
	shortLower := strings.ToLower(shortName)

	// Include beans, models, DTOs, enums, configs
	if strings.Contains(lower, ".beans.") || strings.Contains(lower, ".model.") ||
		strings.Contains(lower, ".dto.") || strings.Contains(lower, ".enums.") {
		return true
	}

	// Include by name pattern
	if strings.HasSuffix(shortLower, "config") || strings.HasSuffix(shortLower, "request") ||
		strings.HasSuffix(shortLower, "response") || strings.HasSuffix(shortLower, "dto") ||
		strings.HasSuffix(shortLower, "entity") || strings.HasSuffix(shortLower, "event") {
		return true
	}

	return false
}

func decompileClass(jarPath, className string) *LibraryClass {
	out, err := exec.Command("javap", "-p", "-classpath", jarPath, className).Output()
	if err != nil {
		return nil
	}

	lines := strings.Split(string(out), "\n")
	parts := strings.Split(className, ".")
	shortName := parts[len(parts)-1]
	pkg := strings.Join(parts[:len(parts)-1], ".")

	lc := &LibraryClass{
		Name:    shortName,
		Package: pkg,
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip empty, class declaration, compiled-from
		if trimmed == "" || trimmed == "}" || strings.HasPrefix(trimmed, "Compiled from") ||
			strings.Contains(trimmed, "class " + shortName) {
			continue
		}

		// Fields: "private final Type name;"
		if (strings.HasPrefix(trimmed, "private ") || strings.HasPrefix(trimmed, "protected ") ||
			strings.HasPrefix(trimmed, "public ")) && strings.HasSuffix(trimmed, ";") && !strings.Contains(trimmed, "(") {
			lc.Fields = append(lc.Fields, simplifyType(trimmed))
			continue
		}

		// Methods: "public Type methodName(...);"
		if strings.Contains(trimmed, "(") && !strings.Contains(trimmed, shortName+"(") {
			if strings.HasPrefix(trimmed, "public ") {
				lc.Methods = append(lc.Methods, simplifyType(trimmed))
			}
		}
	}

	if len(lc.Fields) == 0 && len(lc.Methods) == 0 {
		return nil
	}
	return lc
}

func simplifyType(sig string) string {
	// Shorten fully qualified types: com.example.platform.foo.Bar → Bar
	result := sig
	for {
		idx := strings.Index(result, "com.example.platform.")
		if idx < 0 {
			break
		}
		end := idx
		for end < len(result) && result[end] != ' ' && result[end] != ',' &&
			result[end] != ')' && result[end] != ';' && result[end] != '<' && result[end] != '>' {
			end++
		}
		fqn := result[idx:end]
		parts := strings.Split(fqn, ".")
		short := parts[len(parts)-1]
		result = result[:idx] + short + result[end:]
	}

	// Also shorten java.lang.* and java.util.*
	for _, prefix := range []string{"java.lang.", "java.util."} {
		for strings.Contains(result, prefix) {
			idx := strings.Index(result, prefix)
			end := idx + len(prefix)
			for end < len(result) && result[end] != ' ' && result[end] != ',' &&
				result[end] != ')' && result[end] != ';' && result[end] != '<' && result[end] != '>' {
				end++
			}
			fqn := result[idx:end]
			parts := strings.Split(fqn, ".")
			short := parts[len(parts)-1]
			result = result[:idx] + short + result[end:]
		}
	}

	return result
}
