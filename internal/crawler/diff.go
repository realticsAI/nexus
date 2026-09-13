package crawler

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type FileChange struct {
	Path    string
	Type    ChangeType
	Service string
}

type ChangeType int

const (
	Modified ChangeType = iota
	Added
	Deleted
)

func DetectFileChanges(repoPath string, oldSHA string, storedChecksums map[string]string) []FileChange {
	if oldSHA != "" && countFiles(repoPath) > 10000 {
		return gitDiffChanges(repoPath, oldSHA)
	}
	return filesystemDiff(repoPath, storedChecksums)
}

func filesystemDiff(repoPath string, storedChecksums map[string]string) []FileChange {
	var changes []FileChange
	seen := make(map[string]bool)

	filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == "target" || base == "build" || base == "__pycache__" || base == ".gradle" || base == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !isSourceFile(path) {
			return nil
		}

		rel, _ := filepath.Rel(repoPath, path)
		seen[rel] = true
		newHash := hashFile(path)
		if newHash == "" {
			return nil
		}

		if old, exists := storedChecksums[rel]; !exists {
			changes = append(changes, FileChange{Path: rel, Type: Added})
		} else if old != newHash {
			changes = append(changes, FileChange{Path: rel, Type: Modified})
		}
		return nil
	})

	for rel := range storedChecksums {
		if !seen[rel] {
			changes = append(changes, FileChange{Path: rel, Type: Deleted})
		}
	}
	return changes
}

func gitDiffChanges(repoPath, oldSHA string) []FileChange {
	out, err := exec.Command("git", "-C", repoPath, "diff", "--name-status", oldSHA+"..HEAD").Output()
	if err != nil {
		return nil
	}
	var changes []FileChange
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		var ct ChangeType
		switch parts[0][0] {
		case 'M':
			ct = Modified
		case 'A':
			ct = Added
		case 'D':
			ct = Deleted
		default:
			ct = Modified
		}
		changes = append(changes, FileChange{Path: parts[len(parts)-1], Type: ct})
	}
	return changes
}

func hashFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return fmt.Sprintf("sha256:%x", h.Sum(nil))
}

func isSourceFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".java", ".ts", ".tsx", ".js", ".jsx", ".py",
		".yml", ".yaml", ".json", ".xml", ".toml",
		".properties", ".env", ".gradle", ".kt":
		return true
	}
	return false
}

func countFiles(dir string) int {
	count := 0
	filepath.Walk(dir, func(_ string, info os.FileInfo, _ error) error {
		if info != nil && !info.IsDir() {
			count++
		}
		return nil
	})
	return count
}

func ComputeChecksums(repoPath string) map[string]string {
	checksums := make(map[string]string)
	filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		base := filepath.Base(filepath.Dir(path))
		if base == ".git" || base == "node_modules" || base == "target" {
			return filepath.SkipDir
		}
		if !isSourceFile(path) {
			return nil
		}
		rel, _ := filepath.Rel(repoPath, path)
		if h := hashFile(path); h != "" {
			checksums[rel] = h
		}
		return nil
	})
	return checksums
}
