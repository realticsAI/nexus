package crawler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.java")
	os.WriteFile(path, []byte("public class Foo {}"), 0644)
	h := hashFile(path)
	if h == "" || len(h) < 10 {
		t.Fatalf("expected valid hash, got %q", h)
	}
	h2 := hashFile(path)
	if h != h2 {
		t.Fatal("same file should produce same hash")
	}
}

func TestIsSourceFile(t *testing.T) {
	cases := map[string]bool{
		"Foo.java": true, "app.ts": true, "main.py": true,
		"config.yml": true, "pom.xml": true,
		"image.png": false, "binary.exe": false, "readme.md": false,
	}
	for file, want := range cases {
		got := isSourceFile(file)
		if got != want {
			t.Errorf("isSourceFile(%q) = %v, want %v", file, got, want)
		}
	}
}

func TestFilesystemDiff(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "existing.java"), []byte("class A {}"), 0644)
	os.WriteFile(filepath.Join(dir, "new.java"), []byte("class B {}"), 0644)

	stored := map[string]string{
		"existing.java": "sha256:different",
		"deleted.java":  "sha256:old",
	}
	changes := filesystemDiff(dir, stored)
	if len(changes) != 3 {
		t.Fatalf("expected 3 changes (modified, added, deleted), got %d", len(changes))
	}
}

func TestComputeChecksums(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "App.java"), []byte("class App {}"), 0644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# readme"), 0644)
	checksums := ComputeChecksums(dir)
	if _, ok := checksums["App.java"]; !ok {
		t.Fatal("should include .java files")
	}
	if _, ok := checksums["README.md"]; ok {
		t.Fatal("should not include .md files")
	}
}
