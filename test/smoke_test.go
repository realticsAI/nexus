package test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func nexusBin(t *testing.T) string {
	t.Helper()
	bin := os.Getenv("NEXUS_BIN")
	if bin == "" {
		bin = "../bin/nexus"
	}
	if _, err := os.Stat(bin); err != nil {
		t.Skip("nexus binary not found — run 'make build' first")
	}
	return bin
}

func TestVersion(t *testing.T) {
	out, err := exec.Command(nexusBin(t), "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("version failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "nexus") {
		t.Fatalf("expected version to contain 'nexus', got: %s", out)
	}
}

func TestHelpShowsSubcommands(t *testing.T) {
	out, err := exec.Command(nexusBin(t), "--help").CombinedOutput()
	if err != nil {
		t.Fatalf("help failed: %v\n%s", err, out)
	}
	help := string(out)
	for _, cmd := range []string{"crawl", "analyze", "serve", "work", "pipeline", "status"} {
		if !strings.Contains(help, cmd) {
			t.Errorf("help missing subcommand %q", cmd)
		}
	}
}

func TestStatusRunsClean(t *testing.T) {
	cmd := exec.Command(nexusBin(t), "status")
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "nexus status") {
		t.Fatalf("expected status header, got: %s", out)
	}
}

func TestCrawlNoWorkspaces(t *testing.T) {
	cmd := exec.Command(nexusBin(t), "crawl")
	cmd.Env = append(os.Environ(), "HOME="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("crawl failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "no workspaces configured") {
		t.Fatalf("expected no-workspace message, got: %s", out)
	}
}

func TestWorkRequiresTicketArg(t *testing.T) {
	out, err := exec.Command(nexusBin(t), "work").CombinedOutput()
	if err == nil {
		t.Fatal("work without args should fail")
	}
	if !strings.Contains(string(out), "accepts 1 arg") {
		t.Logf("output: %s", out)
	}
}
