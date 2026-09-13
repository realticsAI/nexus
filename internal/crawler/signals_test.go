package crawler

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCollectSignalsAppYml(t *testing.T) {
	dir := t.TempDir()
	resDir := filepath.Join(dir, "src", "main", "resources")
	os.MkdirAll(resDir, 0755)
	os.WriteFile(filepath.Join(resDir, "application.yml"), []byte(`
server:
  port: 8080
app:
  payment-url: http://payment-svc:8080/api
`), 0644)

	signals := CollectSignals(dir)
	if signals.Config["server.port"] == "" {
		t.Fatal("expected server.port in config")
	}
	if signals.Config["app.payment-url"] != "http://payment-svc:8080/api" {
		t.Fatalf("expected payment URL, got %q", signals.Config["app.payment-url"])
	}
}

func TestCollectSignalsPomXml(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pom.xml"), []byte(`<?xml version="1.0"?>
<project>
  <dependencies>
    <dependency>
      <groupId>org.springframework.boot</groupId>
      <artifactId>spring-boot-starter-web</artifactId>
    </dependency>
    <dependency>
      <groupId>com.myorg</groupId>
      <artifactId>commons-lib</artifactId>
    </dependency>
  </dependencies>
</project>`), 0644)

	signals := CollectSignals(dir)
	if len(signals.BuildDeps) < 2 {
		t.Fatalf("expected at least 2 build deps, got %d", len(signals.BuildDeps))
	}
}

func TestCollectSignalsPackageJSON(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{
  "name": "loyalty-web",
  "version": "2.1.0",
  "dependencies": {
    "react": "^18.2.0",
    "next": "14.0.0",
    "axios": "^1.0.0"
  },
  "devDependencies": {
    "typescript": "^5.3.0",
    "@angular/core": "17.0.0"
  },
  "scripts": {
    "start": "next start --port 3001",
    "dev": "next dev -p 3002"
  }
}`), 0644)

	signals := CollectSignals(dir)
	if len(signals.BuildDeps) != 3 {
		t.Fatalf("expected 3 deps, got %d: %v", len(signals.BuildDeps), signals.BuildDeps)
	}
	if signals.Config["package.name"] != "loyalty-web" {
		t.Fatalf("expected package.name=loyalty-web, got %q", signals.Config["package.name"])
	}
	if signals.Config["package.version"] != "2.1.0" {
		t.Fatalf("expected package.version=2.1.0, got %q", signals.Config["package.version"])
	}
	if signals.Config["framework.react"] != "^18.2.0" {
		t.Fatalf("expected framework.react=^18.2.0, got %q", signals.Config["framework.react"])
	}
	if signals.Config["framework.next"] != "14.0.0" {
		t.Fatalf("expected framework.next=14.0.0, got %q", signals.Config["framework.next"])
	}
	if signals.Config["framework.@angular/core"] != "17.0.0" {
		t.Fatalf("expected framework.@angular/core from devDeps, got %q", signals.Config["framework.@angular/core"])
	}
	if signals.Config["typescript.version"] != "^5.3.0" {
		t.Fatalf("expected typescript.version=^5.3.0, got %q", signals.Config["typescript.version"])
	}
	if signals.Config["package.port"] != "3001" {
		t.Fatalf("expected package.port=3001, got %q", signals.Config["package.port"])
	}
}

func TestExtractPortFromScript(t *testing.T) {
	tests := []struct {
		script string
		want   string
	}{
		{"next start --port 3001", "3001"},
		{"PORT=4000 node server.js", "4000"},
		{"react-scripts start -p 8080", "8080"},
		{"node server.js", ""},
	}
	for _, tt := range tests {
		got := extractPortFromScript(tt.script)
		if got != tt.want {
			t.Errorf("extractPortFromScript(%q) = %q, want %q", tt.script, got, tt.want)
		}
	}
}

func TestCollectSignalsEnvFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".env"), []byte(`
# comment
API_URL=http://localhost:3000
DATABASE_URL=postgres://localhost/db
`), 0644)

	signals := CollectSignals(dir)
	if signals.Config["env.API_URL"] != "http://localhost:3000" {
		t.Fatalf("expected API_URL, got %q", signals.Config["env.API_URL"])
	}
}
