package gitops

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type GitOps struct {
	repoPath     string
	branchPrefix string
}

func New(repoPath, branchPrefix string) *GitOps {
	if branchPrefix == "" {
		branchPrefix = "nexus/"
	}
	return &GitOps{repoPath: repoPath, branchPrefix: branchPrefix}
}

func (g *GitOps) CreateBranch(ticketKey string) (string, error) {
	branch := g.branchPrefix + strings.ToLower(ticketKey)
	if _, err := g.run("git", "checkout", "-b", branch); err != nil {
		return "", fmt.Errorf("creating branch %s: %w", branch, err)
	}
	return branch, nil
}

func (g *GitOps) SwitchBranch(branch string) error {
	_, err := g.run("git", "checkout", branch)
	return err
}

func (g *GitOps) CurrentBranch() (string, error) {
	out, err := g.run("git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (g *GitOps) WriteFile(relPath, content string) error {
	fullPath := filepath.Join(g.repoPath, relPath)
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(fullPath, []byte(content), 0644)
}

func (g *GitOps) ReadFile(relPath string) (string, error) {
	data, err := os.ReadFile(filepath.Join(g.repoPath, relPath))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (g *GitOps) StageFiles(paths []string) error {
	args := append([]string{"add"}, paths...)
	_, err := g.run("git", args...)
	return err
}

func (g *GitOps) Commit(message string) (string, error) {
	out, err := g.run("git", "commit", "-m", message)
	if err != nil {
		return "", fmt.Errorf("commit failed: %w\n%s", err, out)
	}
	sha, _ := g.run("git", "rev-parse", "HEAD")
	return strings.TrimSpace(sha), nil
}

func (g *GitOps) Push(branch string) error {
	_, err := g.run("git", "push", "-u", "origin", branch)
	return err
}

func (g *GitOps) Diff() (string, error) {
	out, err := g.run("git", "diff", "--stat")
	if err != nil {
		return "", err
	}
	return out, nil
}

func (g *GitOps) DiffFull() (string, error) {
	out, err := g.run("git", "diff")
	if err != nil {
		return "", err
	}
	return out, nil
}

func (g *GitOps) FilesChanged() ([]string, error) {
	out, err := g.run("git", "diff", "--name-only")
	if err != nil {
		return nil, err
	}
	staged, err := g.run("git", "diff", "--cached", "--name-only")
	if err != nil {
		return nil, err
	}
	untracked, err := g.run("git", "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	combined := strings.TrimSpace(out) + "\n" + strings.TrimSpace(staged) + "\n" + strings.TrimSpace(untracked)
	seen := make(map[string]bool)
	var files []string
	for _, f := range strings.Split(combined, "\n") {
		f = strings.TrimSpace(f)
		if f != "" && !seen[f] {
			seen[f] = true
			files = append(files, f)
		}
	}
	return files, nil
}

func (g *GitOps) CreatePR(title, body, base string) (string, error) {
	out, err := g.run("gh", "pr", "create", "--title", title, "--body", body, "--base", base)
	if err != nil {
		return "", fmt.Errorf("creating PR: %w\n%s", err, out)
	}
	return strings.TrimSpace(out), nil
}

func (g *GitOps) CheckoutMain() error {
	_, err := g.run("git", "checkout", "main")
	if err != nil {
		_, err = g.run("git", "checkout", "master")
	}
	return err
}

func (g *GitOps) RunBuild(cmd string) (string, error) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty build command")
	}
	return g.run(parts[0], parts[1:]...)
}

func (g *GitOps) RunTests(cmd string) (string, error) {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty test command")
	}
	return g.run(parts[0], parts[1:]...)
}

func (g *GitOps) run(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = g.repoPath
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (g *GitOps) RepoPath() string { return g.repoPath }
