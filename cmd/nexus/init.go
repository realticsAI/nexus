package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

type prereq struct {
	name    string
	bin     string
	brewPkg string
	envLine string // line to add to zshrc if needed
}

func checkPrereqs() (installed []string, missing []prereq) {
	reqs := []prereq{
		{name: "Git", bin: "git", brewPkg: "git"},
		{name: "Go", bin: "go", brewPkg: "go",
			envLine: `export GOPATH="$HOME/go"` + "\n" + `export PATH="$GOPATH/bin:$HOME/bin:$PATH"`},
		{name: "GitHub CLI", bin: "gh", brewPkg: "gh"},
		{name: "AWS CLI", bin: "aws", brewPkg: "awscli"},
	}
	for _, r := range reqs {
		if _, err := exec.LookPath(r.bin); err == nil {
			installed = append(installed, r.name)
		} else {
			missing = append(missing, r)
		}
	}
	return
}

func brewAvailable() bool {
	_, err := exec.LookPath("brew")
	return err == nil
}

func installBrew() error {
	fmt.Println("     Installing Homebrew...")
	cmd := exec.Command("/bin/bash", "-c",
		`/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func brewInstall(pkg string) error {
	cmd := exec.Command("brew", "install", pkg)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func ensureZshrcLines(home string, lines []string) (added []string) {
	zshrc := filepath.Join(home, ".zshrc")
	existing := ""
	if data, err := os.ReadFile(zshrc); err == nil {
		existing = string(data)
	}

	var toAdd []string
	for _, line := range lines {
		for _, sub := range strings.Split(line, "\n") {
			sub = strings.TrimSpace(sub)
			if sub != "" && !strings.Contains(existing, sub) {
				toAdd = append(toAdd, sub)
			}
		}
	}
	if len(toAdd) == 0 {
		return nil
	}

	f, err := os.OpenFile(zshrc, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	w.WriteString("\n# Added by nexus init\n")
	for _, l := range toAdd {
		w.WriteString(l + "\n")
		added = append(added, l)
	}
	w.Flush()
	return added
}

var initCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "First-time setup — install, prerequisites, config, index, and MCP registration",
	Long:  "Interactive setup: self-installs to ~/bin, checks/installs prerequisites (Go, gh, aws), creates config, indexes your codebase, and registers Nexus as an MCP server.\n\nOptionally pass a workspace path as a positional argument (e.g. 'nexus init .' or 'nexus init ~/repos'). Relative paths are resolved to absolute. Falls back to --workspace flag, then ~/Documents/ExampleOrg.",
	RunE: func(cmd *cobra.Command, args []string) error {
		home, _ := os.UserHomeDir()
		nexusHome := filepath.Join(home, ".nexus")
		configPath := filepath.Join(nexusHome, "config.yml")
		binPath, _ := os.Executable()

		fmt.Println("=== Nexus Setup ===")
		fmt.Println()

		// Step 1: Self-install to ~/bin
		fmt.Println("[1/6] Installing nexus to ~/bin...")
		binDir := filepath.Join(home, "bin")
		os.MkdirAll(binDir, 0755)
		installPath := filepath.Join(binDir, "nexus")
		if binPath != installPath {
			srcData, err := os.ReadFile(binPath)
			if err != nil {
				fmt.Printf("     !! Could not read binary: %v\n", err)
			} else if err := os.WriteFile(installPath, srcData, 0755); err != nil {
				fmt.Printf("     !! Could not install to ~/bin: %v\n", err)
			} else {
				fmt.Printf("     Installed to %s\n", installPath)
				binPath = installPath
			}
		} else {
			fmt.Println("     Already running from ~/bin.")
		}

		// Step 2: Prerequisites
		fmt.Println("[2/6] Checking prerequisites...")
		installed, missing := checkPrereqs()
		for _, name := range installed {
			fmt.Printf("     ✓ %s\n", name)
		}

		if len(missing) > 0 {
			if runtime.GOOS != "darwin" {
				fmt.Println("     Not on macOS — install these manually:")
				for _, m := range missing {
					fmt.Printf("     ✗ %s (%s)\n", m.name, m.bin)
				}
				fmt.Println("     Then re-run 'nexus init'.")
			} else {
				if !brewAvailable() {
					fmt.Println("     Homebrew not found — installing...")
					if err := installBrew(); err != nil {
						fmt.Printf("     !! Homebrew install failed: %v\n", err)
						fmt.Println("     Install manually: https://brew.sh then re-run 'nexus init'.")
					} else {
						fmt.Println("     ✓ Homebrew installed")
					}
				}

				if brewAvailable() {
					var envLines []string
					for _, m := range missing {
						fmt.Printf("     Installing %s via brew...\n", m.name)
						if err := brewInstall(m.brewPkg); err != nil {
							fmt.Printf("     ✗ %s install failed: %v\n", m.name, err)
						} else {
							fmt.Printf("     ✓ %s installed\n", m.name)
							if m.envLine != "" {
								envLines = append(envLines, m.envLine)
							}
						}
					}

					// Always ensure ~/bin is on PATH
					envLines = append(envLines, `export PATH="$HOME/bin:$PATH"`)

					added := ensureZshrcLines(home, envLines)
					if len(added) > 0 {
						fmt.Println("     Added to ~/.zshrc:")
						for _, l := range added {
							fmt.Printf("       %s\n", l)
						}
						fmt.Println("     Run 'source ~/.zshrc' or open a new terminal to pick up changes.")
					}
				}
			}
		} else {
			// Even if everything is installed, make sure ~/bin is in PATH in zshrc
			added := ensureZshrcLines(home, []string{`export PATH="$HOME/bin:$PATH"`})
			if len(added) > 0 {
				fmt.Println("     Added ~/bin to PATH in ~/.zshrc")
			}
		}

		// Step 3: Config
		if _, err := os.Stat(configPath); err == nil {
			fmt.Printf("[3/6] Config exists at %s — skipping\n", configPath)
		} else {
			fmt.Printf("[3/6] Creating config at %s\n", configPath)
			os.MkdirAll(nexusHome, 0755)

			// Resolve workspace: positional arg > --workspace flag > default
			workspace, _ := cmd.Flags().GetString("workspace")
			if len(args) > 0 && args[0] != "" {
				workspace = args[0]
			}
			if workspace == "" {
				workspace = "~/Documents/ExampleOrg"
			}
			// Resolve relative paths to absolute (e.g. "." → "/Users/dev/repos")
			if workspace != "" && !strings.HasPrefix(workspace, "~") && !filepath.IsAbs(workspace) {
				if abs, err := filepath.Abs(workspace); err == nil {
					workspace = abs
				}
			}
			profile, _ := cmd.Flags().GetString("profile")
			if profile == "" {
				profile = "AWS-EXAMPLE-BEDROCK-123456789012"
			}
			org, _ := cmd.Flags().GetString("org")
			if org == "" {
				org = "ExampleOrg"
			}

			config := fmt.Sprintf(`workspaces:
  - path: %s

github:
  token_env: GITHUB_TOKEN

llm:
  provider: bedrock
  model: us.anthropic.claude-opus-4-6-v1
  region: us-east-1
  profile: %s
  max_retries: 3

jira:
  base_url: https://jira.example.com
  token_env: HALO_ATLASSIAN_JIRA_PAT

confluence:
  base_url: https://confluence.example.com
  token_env: HALO_ATLASSIAN_CONFLUENCE_PAT

figma:
  token_env: NEXUS_FIGMA_PAT

agent:
  auto_push: false
  auto_transition: false
  max_files_changed: 10
  require_tests: true
  branch_prefix: "nexus/"
  github_org: "%s"

crawl:
  git_pull_concurrency: 20
  parse_concurrency: 8
  embed_concurrency: 2
`, workspace, profile, org)

			if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
				return fmt.Errorf("writing config: %w", err)
			}
			fmt.Printf("     Created. Edit %s to customize.\n", configPath)
		}

		// Step 4: Check env vars
		fmt.Println("[4/6] Checking environment...")
		missingEnv := []string{}
		for _, env := range []string{"HALO_ATLASSIAN_JIRA_PAT"} {
			if os.Getenv(env) == "" {
				missingEnv = append(missingEnv, env)
			}
		}
		optionalEnv := []string{}
		for _, env := range []string{"GITHUB_TOKEN", "NEXUS_FIGMA_PAT", "HALO_ATLASSIAN_CONFLUENCE_PAT"} {
			if os.Getenv(env) == "" {
				optionalEnv = append(optionalEnv, env)
			}
		}
		if len(missingEnv) > 0 {
			fmt.Printf("     !! Required: %s\n", strings.Join(missingEnv, ", "))
			fmt.Println("     Set these in ~/.zshrc and re-run.")
		} else {
			fmt.Println("     All required env vars set.")
		}
		if len(optionalEnv) > 0 {
			fmt.Printf("     Optional (not set): %s\n", strings.Join(optionalEnv, ", "))
		}

		// Step 4: Index
		skipCrawl, _ := cmd.Flags().GetBool("skip-crawl")
		if skipCrawl {
			fmt.Println("[5/6] Skipping crawl (--skip-crawl)")
		} else {
			fmt.Println("[5/6] Indexing codebase (this may take a minute)...")
			crawlCmd := exec.Command(binPath, "crawl")
			crawlCmd.Stdout = os.Stderr
			crawlCmd.Stderr = os.Stderr
			if err := crawlCmd.Run(); err != nil {
				fmt.Printf("     !! Crawl failed: %v\n", err)
				fmt.Println("     You can run 'nexus crawl' later.")
			} else {
				fmt.Println("     Index complete.")
			}
		}

		// Step 6: Register MCP
		fmt.Println("[6/6] Registering MCP server...")
		agentCLI, err := exec.LookPath("claude")
		if err != nil {
			fmt.Println("     No MCP-compatible agent CLI found.")
			fmt.Printf("     Register manually with your AI agent's MCP config: %s serve\n", binPath)
		} else {
			registerCmd := exec.Command(agentCLI, "mcp", "add", "nexus", "--scope", "user", "--", binPath, "serve")
			out, err := registerCmd.CombinedOutput()
			if err != nil {
				if strings.Contains(string(out), "already exists") {
					fmt.Println("     Already registered.")
				} else {
					fmt.Printf("     Registration failed: %s\n", strings.TrimSpace(string(out)))
					fmt.Printf("     Register manually with your AI agent's MCP config: %s serve\n", binPath)
				}
			} else {
				fmt.Println("     Registered globally for all projects.")
			}
		}

		fmt.Println()
		fmt.Println("=== Ready ===")
		fmt.Println()
		fmt.Println("Nexus MCP is registered with 16 tools:")
		fmt.Println()
		fmt.Println("  Code Intelligence: search, grep, find_endpoints, read_file,")
		fmt.Println("    list_services, service_profile, trace_dependencies,")
		fmt.Println("    trace_dependents, impact_analysis, status, review_pr_context")
		fmt.Println()
		fmt.Println("  Agent Pipeline:")
		fmt.Println("    investigate  — scan codebase for a Jira ticket (no LLM)")
		fmt.Println("    analyze      — extract requirements via LLM")
		fmt.Println("    plan         — generate implementation plan")
		fmt.Println("    validate     — full pipeline with validation")
		fmt.Println("    handoff      — agent-ready implementation prompt")
		fmt.Println()
		fmt.Println("Start a new agent session and try:")
		fmt.Println("  \"Use nexus to investigate PROJ-1001\"")
		fmt.Println()
		return nil
	},
}

func init() {
	initCmd.Flags().String("workspace", "", "Workspace path (default: ~/Documents/ExampleOrg)")
	initCmd.Flags().String("profile", "", "AWS Bedrock profile name")
	initCmd.Flags().String("org", "", "GitHub org for file fallback")
	initCmd.Flags().Bool("skip-crawl", false, "Skip codebase indexing")
	rootCmd.AddCommand(initCmd)
}
