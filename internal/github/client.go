package github

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func FetchFileContent(repoPath, filePath, org string) ([]byte, error) {
	repo := extractRepoName(repoPath)
	if repo == "" {
		return nil, fmt.Errorf("cannot derive repo name from path: %s", repoPath)
	}

	owner := org
	if owner == "" {
		owner = extractOrgFromPath(repoPath)
	}
	if owner == "" {
		return nil, fmt.Errorf("cannot determine GitHub org for %s", repoPath)
	}

	apiPath := fmt.Sprintf("repos/%s/%s/contents/%s", owner, repo, filePath)
	out, err := exec.Command("gh", "api", apiPath, "--jq", ".content").Output()
	if err != nil {
		return nil, fmt.Errorf("gh api %s: %w", apiPath, err)
	}

	content := strings.TrimSpace(string(out))
	decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(content, "\n", ""))
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}

	return decoded, nil
}

func FetchFileContentBatch(repoPath string, filePaths []string, org string) map[string][]byte {
	results := make(map[string][]byte)
	for _, fp := range filePaths {
		data, err := FetchFileContent(repoPath, fp, org)
		if err == nil {
			results[fp] = data
		}
	}
	return results
}

func RepoExists(repoPath, org string) bool {
	repo := extractRepoName(repoPath)
	if repo == "" || org == "" {
		return false
	}
	apiPath := fmt.Sprintf("repos/%s/%s", org, repo)
	out, err := exec.Command("gh", "api", apiPath, "--jq", ".full_name").Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func ListRepoFiles(repoPath, dirPath, org string) ([]string, error) {
	repo := extractRepoName(repoPath)
	if repo == "" {
		return nil, fmt.Errorf("cannot derive repo name from path: %s", repoPath)
	}
	owner := org
	if owner == "" {
		owner = extractOrgFromPath(repoPath)
	}

	apiPath := fmt.Sprintf("repos/%s/%s/contents/%s", owner, repo, dirPath)
	out, err := exec.Command("gh", "api", apiPath).Output()
	if err != nil {
		return nil, fmt.Errorf("gh api %s: %w", apiPath, err)
	}

	var items []struct {
		Name string `json:"name"`
		Type string `json:"type"`
		Path string `json:"path"`
	}
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, err
	}

	var files []string
	for _, item := range items {
		files = append(files, item.Path)
	}
	return files, nil
}

func extractRepoName(repoPath string) string {
	clean := filepath.Clean(repoPath)
	return filepath.Base(clean)
}

func extractOrgFromPath(repoPath string) string {
	clean := filepath.Clean(repoPath)
	parent := filepath.Base(filepath.Dir(clean))
	if parent == "." || parent == "/" {
		return ""
	}
	return parent
}
