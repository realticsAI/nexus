package crawler

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func createTestRepo(t *testing.T, dir, name string) string {
	t.Helper()
	repoPath := filepath.Join(dir, name)
	os.MkdirAll(repoPath, 0755)
	exec.Command("git", "-C", repoPath, "init", "--initial-branch=main").Run()
	exec.Command("git", "-C", repoPath, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", repoPath, "config", "user.name", "Test").Run()
	os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("# test"), 0644)
	exec.Command("git", "-C", repoPath, "add", ".").Run()
	exec.Command("git", "-C", repoPath, "commit", "-m", "init").Run()
	return repoPath
}

func TestDiscoverRepos(t *testing.T) {
	dir := t.TempDir()
	createTestRepo(t, dir, "service-a")
	createTestRepo(t, dir, "service-b")
	os.MkdirAll(filepath.Join(dir, "not-a-repo"), 0755)

	repos := DiscoverRepos([]string{dir})
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}
	for _, r := range repos {
		if r.Workspace == "" {
			t.Error("workspace should not be empty")
		}
	}
}

func TestDiscoverRepos_Nested(t *testing.T) {
	dir := t.TempDir()
	// Top-level repos
	createTestRepo(t, dir, "service-a")

	// Nested group: dir/order_service/order.service (git repo inside a non-git parent)
	groupDir := filepath.Join(dir, "order_service")
	os.MkdirAll(groupDir, 0755)
	createTestRepo(t, groupDir, "order.service")
	createTestRepo(t, groupDir, "order.service-events")
	createTestRepo(t, groupDir, "content-services.app-hub")

	// Another nested group
	gqlDir := filepath.Join(dir, "graphql")
	os.MkdirAll(gqlDir, 0755)
	createTestRepo(t, gqlDir, "core.voyages-graphql")
	createTestRepo(t, gqlDir, "commerce.cart-graphql")

	// Non-repo dir (should be skipped)
	os.MkdirAll(filepath.Join(dir, "docs"), 0755)

	repos := DiscoverRepos([]string{dir})
	// 1 top-level + 3 under order_service + 2 under graphql = 6
	if len(repos) != 6 {
		t.Fatalf("expected 6 repos, got %d: %v", len(repos), names(repos))
	}

	nameSet := make(map[string]bool)
	for _, r := range repos {
		nameSet[r.Name] = true
		if r.Workspace == "" {
			t.Errorf("workspace should not be empty for %s", r.Name)
		}
	}

	for _, expected := range []string{"service-a", "order.service", "order.service-events", "content-services.app-hub", "core.voyages-graphql", "commerce.cart-graphql"} {
		if !nameSet[expected] {
			t.Errorf("missing expected repo %s, found: %v", expected, names(repos))
		}
	}
}

func TestDiscoverRepos_NoDuplicates(t *testing.T) {
	dir := t.TempDir()
	createTestRepo(t, dir, "service-a")

	// Pass the same dir twice — should not duplicate
	repos := DiscoverRepos([]string{dir, dir})
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo (deduped), got %d", len(repos))
	}
}

func TestServiceKey(t *testing.T) {
	key := ServiceKey("backend", "orders")
	if key != "backend:orders" {
		t.Fatalf("expected backend:orders, got %s", key)
	}
}

func TestGetHeadSHA(t *testing.T) {
	dir := t.TempDir()
	repoPath := createTestRepo(t, dir, "test-repo")
	sha := GetHeadSHA(repoPath)
	if sha == "" || len(sha) < 7 {
		t.Fatalf("expected valid SHA, got %q", sha)
	}
}

func TestExpandMonorepo_Maven(t *testing.T) {
	dir := t.TempDir()
	repo := RepoInfo{Path: dir, Name: "backend", Workspace: "ws"}

	os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(`<project>
  <modules>
    <module>service-auth</module>
    <module>service-orders</module>
    <module>common-lib</module>
  </modules>
</project>`), 0644)

	os.MkdirAll(filepath.Join(dir, "service-auth"), 0755)
	os.MkdirAll(filepath.Join(dir, "service-orders"), 0755)
	os.MkdirAll(filepath.Join(dir, "common-lib"), 0755)

	result := ExpandMonorepo(repo)
	// root + 3 modules
	if len(result) != 4 {
		t.Fatalf("expected 4 repos (root + 3 modules), got %d", len(result))
	}
	if result[0].Name != "backend" {
		t.Errorf("first entry should be root repo, got %s", result[0].Name)
	}
	if result[1].Name != "backend/service-auth" {
		t.Errorf("expected backend/service-auth, got %s", result[1].Name)
	}
}

func TestExpandMonorepo_TSWorkspaces(t *testing.T) {
	dir := t.TempDir()
	repo := RepoInfo{Path: dir, Name: "mono-ts", Workspace: "ws"}

	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{
  "name": "mono-ts",
  "workspaces": ["packages/*", "apps/web"]
}`), 0644)

	os.MkdirAll(filepath.Join(dir, "packages", "ui"), 0755)
	os.MkdirAll(filepath.Join(dir, "packages", "utils"), 0755)
	os.MkdirAll(filepath.Join(dir, "apps", "web"), 0755)

	result := ExpandMonorepo(repo)
	// root + packages/ui + packages/utils + apps/web
	if len(result) != 4 {
		t.Fatalf("expected 4 repos, got %d: %v", len(result), names(result))
	}
}

func TestExpandMonorepo_Gradle(t *testing.T) {
	dir := t.TempDir()
	repo := RepoInfo{Path: dir, Name: "android-app", Workspace: "ws"}

	os.WriteFile(filepath.Join(dir, "settings.gradle.kts"), []byte(`
rootProject.name = "android-app"
include(":app")
include(":core:network")
include(":feature:home")
`), 0644)

	os.MkdirAll(filepath.Join(dir, "app"), 0755)
	os.MkdirAll(filepath.Join(dir, "core", "network"), 0755)
	os.MkdirAll(filepath.Join(dir, "feature", "home"), 0755)

	result := ExpandMonorepo(repo)
	if len(result) != 4 {
		t.Fatalf("expected 4 repos, got %d: %v", len(result), names(result))
	}
}

func TestExpandMonorepo_SingleModule(t *testing.T) {
	dir := t.TempDir()
	repo := RepoInfo{Path: dir, Name: "simple", Workspace: "ws"}

	// No pom.xml, no package.json, no settings.gradle
	result := ExpandMonorepo(repo)
	if len(result) != 1 {
		t.Fatalf("expected 1 repo (unchanged), got %d", len(result))
	}
	if result[0].Name != "simple" {
		t.Errorf("expected simple, got %s", result[0].Name)
	}
}

func TestExpandMonorepo_PnpmWorkspaces(t *testing.T) {
	dir := t.TempDir()
	repo := RepoInfo{Path: dir, Name: "pnpm-mono", Workspace: "ws"}

	os.WriteFile(filepath.Join(dir, "pnpm-workspace.yaml"), []byte(`packages:
  - "apps/*"
  - "libs/*"
`), 0644)

	os.MkdirAll(filepath.Join(dir, "apps", "frontend"), 0755)
	os.MkdirAll(filepath.Join(dir, "libs", "shared"), 0755)

	result := ExpandMonorepo(repo)
	if len(result) != 3 {
		t.Fatalf("expected 3 repos, got %d: %v", len(result), names(result))
	}
}

func names(repos []RepoInfo) []string {
	out := make([]string, len(repos))
	for i, r := range repos {
		out[i] = r.Name
	}
	return out
}
