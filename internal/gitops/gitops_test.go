package gitops

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("setup %v failed: %v\n%s", args, err, out)
		}
	}
	run("git", "init")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test"), 0644)
	run("git", "add", ".")
	run("git", "commit", "-m", "initial")
	return dir
}

func TestCreateBranch(t *testing.T) {
	dir := setupTestRepo(t)
	g := New(dir, "nexus/")
	branch, err := g.CreateBranch("PROJ-123")
	if err != nil {
		t.Fatal(err)
	}
	if branch != "nexus/proj-123" {
		t.Fatalf("expected nexus/proj-123, got %s", branch)
	}
	current, _ := g.CurrentBranch()
	if current != "nexus/proj-123" {
		t.Fatalf("expected to be on nexus/proj-123, got %s", current)
	}
}

func TestWriteAndReadFile(t *testing.T) {
	dir := setupTestRepo(t)
	g := New(dir, "nexus/")
	err := g.WriteFile("src/Main.java", "public class Main {}")
	if err != nil {
		t.Fatal(err)
	}
	content, err := g.ReadFile("src/Main.java")
	if err != nil {
		t.Fatal(err)
	}
	if content != "public class Main {}" {
		t.Fatalf("wrong content: %s", content)
	}
}

func TestCommit(t *testing.T) {
	dir := setupTestRepo(t)
	g := New(dir, "nexus/")
	g.CreateBranch("PROJ-456")
	g.WriteFile("test.txt", "hello")
	g.StageFiles([]string{"test.txt"})
	sha, err := g.Commit("test: add test file")
	if err != nil {
		t.Fatal(err)
	}
	if len(sha) < 7 {
		t.Fatalf("expected SHA, got %s", sha)
	}
}

func TestSafetyMaxFiles(t *testing.T) {
	dir := setupTestRepo(t)
	g := New(dir, "nexus/")
	for i := 0; i < 15; i++ {
		g.WriteFile(fmt.Sprintf("file%d.txt", i), "content")
	}
	safety := SafetyCheck{MaxFilesChanged: 10}
	violations, err := safety.Validate(g)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) == 0 {
		t.Fatal("expected violation for too many files")
	}
}

func TestFilesChanged(t *testing.T) {
	dir := setupTestRepo(t)
	g := New(dir, "nexus/")
	g.WriteFile("new.txt", "content")
	g.StageFiles([]string{"new.txt"})
	files, err := g.FilesChanged()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("expected at least 1 file changed")
	}
}
