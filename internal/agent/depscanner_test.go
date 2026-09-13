package agent

import (
	"os"
	"testing"
)

func TestScanLibraryAPIs_WithMavenProperties(t *testing.T) {
	repoPath := os.Getenv("NEXUS_TEST_REPO")
	if repoPath == "" {
		t.Skip("set NEXUS_TEST_REPO to a Maven repo path to run this test")
	}

	apis := ScanLibraryAPIs(repoPath)
	if len(apis) == 0 {
		t.Error("expected at least one library API from repo dependencies")
	}
	for _, api := range apis {
		if len(api.Classes) == 0 {
			t.Errorf("library %s:%s:%s has no classes", api.GroupID, api.ArtifactID, api.Version)
		}
	}
}

func TestIsInternalGroup(t *testing.T) {
	tests := []struct {
		group    string
		expected bool
	}{
		{"com.example.platform.commons", true},
		{"com.example.platform.rewards", true},
		{"com.example.middleware.commons", true},
		{"com.example.telemetry", true},
		{"com.example.middleware", true},
		{"org.springframework.boot", false},
		{"com.fasterxml.jackson", false},
	}
	for _, tt := range tests {
		if got := isInternalGroup(tt.group); got != tt.expected {
			t.Errorf("isInternalGroup(%q) = %v, want %v", tt.group, got, tt.expected)
		}
	}
}

func TestResolveProperty(t *testing.T) {
	props := map[string]string{
		"commons.version": "3.5.3",
		"java.version":    "17",
	}
	tests := []struct {
		input    string
		expected string
	}{
		{"${commons.version}", "3.5.3"},
		{"1.0.0", "1.0.0"},
		{"${missing.prop}", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := resolveProperty(tt.input, props); got != tt.expected {
			t.Errorf("resolveProperty(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
