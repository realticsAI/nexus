package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Platform int

const (
	Java Platform = iota
	TypeScript
	Python
	Swift
	Kotlin
	Unknown
)

var platformNames = map[Platform]string{
	Java: "java", TypeScript: "typescript", Python: "python", Swift: "swift", Kotlin: "kotlin", Unknown: "unknown",
}
var platformValues = map[string]Platform{
	"java": Java, "typescript": TypeScript, "python": Python, "swift": Swift, "kotlin": Kotlin, "unknown": Unknown,
}

func (p Platform) String() string {
	if s, ok := platformNames[p]; ok {
		return s
	}
	return "unknown"
}

func ParsePlatform(s string) Platform {
	if p, ok := platformValues[s]; ok {
		return p
	}
	return Unknown
}

func (p Platform) MarshalJSON() ([]byte, error) {
	return json.Marshal(p.String())
}

func (p *Platform) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*p = ParsePlatform(s)
	return nil
}

func DetectPlatform(repoPath string) Platform {
	if fileExists(filepath.Join(repoPath, "pom.xml")) || fileExists(filepath.Join(repoPath, "build.gradle")) {
		return Java
	}
	if fileExists(filepath.Join(repoPath, "package.json")) {
		return TypeScript
	}
	if fileExists(filepath.Join(repoPath, "pyproject.toml")) || fileExists(filepath.Join(repoPath, "requirements.txt")) || fileExists(filepath.Join(repoPath, "setup.py")) {
		return Python
	}
	if fileExists(filepath.Join(repoPath, "Package.swift")) || hasChildWithExt(repoPath, ".xcodeproj") || hasChildWithExt(repoPath, ".xcworkspace") {
		return Swift
	}
	if fileExists(filepath.Join(repoPath, "build.gradle.kts")) || hasChildWithFile(repoPath, "build.gradle.kts") {
		return Kotlin
	}
	if hasChildWithFile(repoPath, "pom.xml") || hasChildWithFile(repoPath, "build.gradle") {
		return Java
	}
	if hasChildWithFile(repoPath, "package.json") {
		return TypeScript
	}
	return Unknown
}

func hasChildWithFile(dir, filename string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			if fileExists(filepath.Join(dir, e.Name(), filename)) {
				return true
			}
		}
	}
	return false
}

func hasChildWithExt(dir, ext string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ext) {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
