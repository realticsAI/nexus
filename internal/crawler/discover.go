package crawler

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type RepoInfo struct {
	Path      string
	Name      string
	Workspace string
	Origin    string
}

func DiscoverRepos(workspaceDirs []string) []RepoInfo {
	var repos []RepoInfo
	seen := make(map[string]bool)
	for _, dir := range workspaceDirs {
		discoverReposRecursive(dir, filepath.Base(dir), 0, 3, seen, &repos)
	}
	return repos
}

func discoverReposRecursive(dir, workspace string, depth, maxDepth int, seen map[string]bool, repos *[]RepoInfo) {
	if depth >= maxDepth {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "target" || name == "build" || name == "vendor" {
			continue
		}
		childPath := filepath.Join(dir, name)
		if seen[childPath] {
			continue
		}
		if isGitRepo(childPath) {
			seen[childPath] = true
			*repos = append(*repos, RepoInfo{
				Path:      childPath,
				Name:      name,
				Workspace: workspace,
				Origin:    getGitRemoteOrigin(childPath),
			})
		} else {
			discoverReposRecursive(childPath, workspace, depth+1, maxDepth, seen, repos)
		}
	}
}

func isGitRepo(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil && info.IsDir()
}

func getGitRemoteOrigin(repoPath string) string {
	out, err := exec.Command("git", "-C", repoPath, "config", "--get", "remote.origin.url").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func ServiceKey(workspace, name string) string {
	return workspace + ":" + name
}

// ExpandMonorepo checks if a repo is a monorepo and returns one RepoInfo per
// sub-project. If it's a single-project repo, returns the original as-is.
func ExpandMonorepo(repo RepoInfo) []RepoInfo {
	var subDirs []string

	subDirs = append(subDirs, detectMavenModules(repo.Path)...)
	subDirs = append(subDirs, detectGradleModules(repo.Path)...)
	subDirs = append(subDirs, detectTSWorkspaces(repo.Path)...)
	subDirs = append(subDirs, detectSwiftSubprojects(repo.Path)...)

	subDirs = dedup(subDirs)

	// Filter to dirs that actually exist
	var valid []string
	for _, d := range subDirs {
		full := filepath.Join(repo.Path, d)
		if info, err := os.Stat(full); err == nil && info.IsDir() {
			valid = append(valid, d)
		}
	}

	if len(valid) == 0 {
		return []RepoInfo{repo}
	}

	subs := make([]RepoInfo, 0, len(valid)+1)
	// Keep the root repo so top-level files are still indexed
	subs = append(subs, repo)
	for _, d := range valid {
		subs = append(subs, RepoInfo{
			Path:      filepath.Join(repo.Path, d),
			Name:      repo.Name + "/" + d,
			Workspace: repo.Workspace,
			Origin:    repo.Origin,
		})
	}
	return subs
}

// --- Maven ---

var mavenModuleRe = regexp.MustCompile(`<module>\s*([^<]+)\s*</module>`)

func detectMavenModules(repoPath string) []string {
	data, err := os.ReadFile(filepath.Join(repoPath, "pom.xml"))
	if err != nil {
		return nil
	}
	if !strings.Contains(string(data), "<modules>") {
		return nil
	}
	matches := mavenModuleRe.FindAllStringSubmatch(string(data), -1)
	var mods []string
	for _, m := range matches {
		name := strings.TrimSpace(m[1])
		if name != "" {
			mods = append(mods, name)
		}
	}
	return mods
}

// --- Gradle (Groovy + Kotlin DSL) ---

var gradleIncludeRe = regexp.MustCompile(`include\s*[\('"]+\s*:?([^'"\)]+)`)

func detectGradleModules(repoPath string) []string {
	var content string
	for _, name := range []string{"settings.gradle.kts", "settings.gradle"} {
		data, err := os.ReadFile(filepath.Join(repoPath, name))
		if err == nil {
			content = string(data)
			break
		}
	}
	if content == "" {
		return nil
	}
	matches := gradleIncludeRe.FindAllStringSubmatch(content, -1)
	var mods []string
	for _, m := range matches {
		mod := strings.TrimSpace(m[1])
		mod = strings.ReplaceAll(mod, ":", "/")
		mod = strings.Trim(mod, "/ ")
		if mod != "" {
			mods = append(mods, mod)
		}
	}
	return mods
}

// --- TypeScript / React Native workspaces ---

func detectTSWorkspaces(repoPath string) []string {
	// package.json workspaces
	if dirs := detectPackageJSONWorkspaces(repoPath); len(dirs) > 0 {
		return dirs
	}
	// lerna.json
	if dirs := detectLernaPackages(repoPath); len(dirs) > 0 {
		return dirs
	}
	// pnpm-workspace.yaml
	if dirs := detectPnpmWorkspaces(repoPath); len(dirs) > 0 {
		return dirs
	}
	return nil
}

func detectPackageJSONWorkspaces(repoPath string) []string {
	data, err := os.ReadFile(filepath.Join(repoPath, "package.json"))
	if err != nil {
		return nil
	}

	// workspaces can be []string or {"packages": []string}
	var raw struct {
		Workspaces json.RawMessage `json:"workspaces"`
	}
	if err := json.Unmarshal(data, &raw); err != nil || raw.Workspaces == nil {
		return nil
	}

	var patterns []string
	if err := json.Unmarshal(raw.Workspaces, &patterns); err != nil {
		var obj struct {
			Packages []string `json:"packages"`
		}
		if err := json.Unmarshal(raw.Workspaces, &obj); err != nil {
			return nil
		}
		patterns = obj.Packages
	}
	return expandGlobPatterns(repoPath, patterns)
}

func detectLernaPackages(repoPath string) []string {
	data, err := os.ReadFile(filepath.Join(repoPath, "lerna.json"))
	if err != nil {
		return nil
	}
	var lerna struct {
		Packages []string `json:"packages"`
	}
	if err := json.Unmarshal(data, &lerna); err != nil {
		return nil
	}
	return expandGlobPatterns(repoPath, lerna.Packages)
}

func detectPnpmWorkspaces(repoPath string) []string {
	data, err := os.ReadFile(filepath.Join(repoPath, "pnpm-workspace.yaml"))
	if err != nil {
		return nil
	}
	// Simple line-based parse: lines starting with "  - " under "packages:"
	var patterns []string
	inPackages := false
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "packages:" {
			inPackages = true
			continue
		}
		if inPackages {
			if strings.HasPrefix(trimmed, "- ") {
				pat := strings.TrimPrefix(trimmed, "- ")
				pat = strings.Trim(pat, "\"' ")
				if pat != "" {
					patterns = append(patterns, pat)
				}
			} else if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
				break
			}
		}
	}
	return expandGlobPatterns(repoPath, patterns)
}

// expandGlobPatterns resolves workspace patterns like "packages/*" into actual directories.
func expandGlobPatterns(repoPath string, patterns []string) []string {
	var dirs []string
	for _, pat := range patterns {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		if strings.Contains(pat, "*") {
			// Single-level wildcard: list dirs in the parent
			parent := strings.TrimSuffix(pat, "/*")
			parent = strings.TrimSuffix(parent, "/**/")
			parentPath := filepath.Join(repoPath, parent)
			entries, err := os.ReadDir(parentPath)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
					dirs = append(dirs, filepath.Join(parent, e.Name()))
				}
			}
		} else {
			dirs = append(dirs, pat)
		}
	}
	return dirs
}

// --- Swift (xcworkspace) ---

var xcFileRefRe = regexp.MustCompile(`location\s*=\s*"group:([^"]+)"`)

func detectSwiftSubprojects(repoPath string) []string {
	// Find .xcworkspace dirs
	entries, err := os.ReadDir(repoPath)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".xcworkspace") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(repoPath, e.Name(), "contents.xcworkspacedata"))
		if err != nil {
			continue
		}
		matches := xcFileRefRe.FindAllStringSubmatch(string(data), -1)
		if len(matches) <= 1 {
			continue
		}
		var dirs []string
		for _, m := range matches {
			ref := strings.TrimSpace(m[1])
			if ref != "" && ref != "." {
				// Strip trailing .xcodeproj if present
				ref = strings.TrimSuffix(ref, ".xcodeproj")
				dirs = append(dirs, ref)
			}
		}
		if len(dirs) > 0 {
			return dirs
		}
	}
	return nil
}

func dedup(in []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
