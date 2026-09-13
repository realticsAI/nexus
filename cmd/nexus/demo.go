package main

import (
	"bufio"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/anurag/nexus/internal/analyzer"
	"github.com/anurag/nexus/internal/config"
	"github.com/anurag/nexus/internal/crawler"
	"github.com/anurag/nexus/internal/jira"
	"github.com/anurag/nexus/internal/store"
	"github.com/anurag/nexus/model"
	"github.com/spf13/cobra"
)

//go:embed embed/demo.html
var demoFS embed.FS

var (
	activeProcMu sync.Mutex
	activeProc   *os.Process
)

var demoCmd = &cobra.Command{
	Use:   "demo",
	Short: "Launch live dashboard in browser",
	Long:  "Starts a local web server with sprint board, service map, pipeline runner, and MCP monitor.",
	RunE: func(cmd *cobra.Command, args []string) error {
		port, _ := cmd.Flags().GetString("port")
		dev, _ := cmd.Flags().GetBool("dev")
		addr := ":" + port

		cfg, err := config.Load()
		if err != nil {
			cfg = config.Defaults()
		}
		s := store.NewStore(cfg.StateDir())

		services, _ := s.LoadAllServices()
		graph, _ := s.LoadGraph()
		if graph == nil {
			graph = &model.Graph{}
		}

		candidates := []string{
			"cmd/nexus/embed/demo.html",
			filepath.Join(filepath.Dir(os.Args[0]), "..", "cmd/nexus/embed/demo.html"),
		}
		if dev {
			for _, c := range candidates {
				if abs, err := filepath.Abs(c); err == nil {
					if _, err := os.Stat(abs); err == nil {
						devHTMLPath = abs
						break
					}
				}
			}
			if devHTMLPath != "" {
				fmt.Fprintf(os.Stderr, "nexus demo: dev mode — serving %s (edit & refresh, no rebuild)\n", devHTMLPath)
			} else {
				fmt.Fprintf(os.Stderr, "nexus demo: --dev flag set but HTML not found on disk, using embedded\n")
			}
		}

		jiraClient := jira.NewClient(cfg.Jira.BaseURL, cfg.Jira.TokenEnv)

		mux := http.NewServeMux()

		mux.HandleFunc("/", serveDashboard)
		mux.HandleFunc("/api/status", handleStatus(cfg, services, graph))
		mux.HandleFunc("/api/graph", handleGraph(services, graph))
		mux.HandleFunc("/api/sprint", handleSprint(jiraClient, services))
		mux.HandleFunc("/api/ticket", handleTicket(jiraClient))
		mux.HandleFunc("/api/run", handleRun)
		mux.HandleFunc("/api/stop", handleStop)
		mux.HandleFunc("/api/demo", handleDemo)
		mux.HandleFunc("/api/mcp/events", handleMCPEvents(cfg))
		mux.HandleFunc("/api/reindex", handleReindex(cfg, s, &services, &graph))

		url := fmt.Sprintf("http://localhost%s", addr)
		fmt.Fprintf(os.Stderr, "nexus demo: starting at %s\n", url)

		go func() {
			time.Sleep(500 * time.Millisecond)
			openBrowser(url)
		}()

		return http.ListenAndServe(addr, mux)
	},
}

var devHTMLPath string

func init() {
	demoCmd.Flags().String("port", "3939", "Port for demo server")
	demoCmd.Flags().Bool("dev", false, "Serve HTML from disk (live reload, no rebuild needed)")
	rootCmd.AddCommand(demoCmd)
}

func serveDashboard(w http.ResponseWriter, r *http.Request) {
	var data []byte
	var err error

	if devHTMLPath != "" {
		data, err = os.ReadFile(devHTMLPath)
	} else {
		data, err = demoFS.ReadFile("embed/demo.html")
	}

	if err != nil {
		http.Error(w, "dashboard not found", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Write(data)
}

func handleStatus(cfg *config.Config, services map[string]*model.ServiceIndex, graph *model.Graph) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		totalUnits := 0
		totalEndpoints := 0
		for _, svc := range services {
			totalUnits += len(svc.Units)
			for _, u := range svc.Units {
				totalEndpoints += len(u.Endpoints)
			}
		}

		llmName := cfg.LLM.Provider
		if cfg.LLM.Model != "" {
			parts := strings.Split(cfg.LLM.Model, ".")
			llmName += " / " + parts[len(parts)-1]
		}

		resp := map[string]any{
			"version":   version,
			"services":  len(services),
			"edges":     len(graph.Edges),
			"tools":     14,
			"llm":       llmName,
			"units":     totalUnits,
			"endpoints": totalEndpoints,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

// ── Reindex API ─────────────────────────────────────────────────────

func handleReindex(cfg *config.Config, s *store.Store, servicesPtr *map[string]*model.ServiceIndex, graphPtr **model.Graph) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		start := time.Now()

		c := crawler.NewCrawler(cfg, s)
		result, err := c.Run(false)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]any{"error": fmt.Sprintf("crawl failed: %v", err)})
			return
		}

		newServices, err := s.LoadAllServices()
		if err != nil {
			json.NewEncoder(w).Encode(map[string]any{"error": fmt.Sprintf("load failed: %v", err)})
			return
		}

		a := analyzer.NewAnalyzer(newServices)
		newGraph := a.BuildGraph()
		if err := s.SaveGraph(newGraph); err != nil {
			json.NewEncoder(w).Encode(map[string]any{"error": fmt.Sprintf("save graph failed: %v", err)})
			return
		}

		*servicesPtr = newServices
		*graphPtr = newGraph

		json.NewEncoder(w).Encode(map[string]any{
			"status":   "ok",
			"indexed":  result.Indexed,
			"services": len(newServices),
			"edges":    len(newGraph.Edges),
			"duration": time.Since(start).Round(time.Millisecond).String(),
		})
	}
}

// ── Service Map API ──────────────────────────────────────────────────

func handleGraph(services map[string]*model.ServiceIndex, graph *model.Graph) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		detail := r.URL.Query().Get("detail")

		nodes := make([]map[string]any, 0, len(services))
		for key, svc := range services {
			endpoints := 0
			classes := 0
			stereotypes := map[string]int{}
			kafkaTopics := map[string][]string{}
			for _, u := range svc.Units {
				classes++
				endpoints += len(u.Endpoints)
				if u.Stereotype != "" {
					stereotypes[u.Stereotype]++
				}
				for _, t := range u.KafkaProduces {
					kafkaTopics["produces"] = append(kafkaTopics["produces"], t)
				}
				for _, t := range u.KafkaConsumes {
					kafkaTopics["consumes"] = append(kafkaTopics["consumes"], t)
				}
			}
			name := key
			if i := strings.LastIndex(key, ":"); i >= 0 {
				name = key[i+1:]
			}
			node := map[string]any{
				"id":          key,
				"name":        name,
				"platform":    svc.Platform.String(),
				"classes":     classes,
				"endpoints":   endpoints,
				"stereotypes": stereotypes,
				"build_deps":  svc.BuildDeps,
			}
			if len(kafkaTopics) > 0 {
				node["kafka"] = kafkaTopics
			}

			if detail == key || detail == name {
				groups := map[string][]map[string]any{}
				for _, u := range svc.Units {
					if u.Stereotype == "" {
						continue
					}
					entry := map[string]any{"name": u.Name, "file": u.File}
					if len(u.Endpoints) > 0 {
						entry["endpoints"] = u.Endpoints
					}
					if len(u.Dependencies) > 0 {
						entry["deps"] = u.Dependencies
					}
					groups[u.Stereotype] = append(groups[u.Stereotype], entry)
				}
				node["detail"] = groups
			}

			nodes = append(nodes, node)
		}

		nodeIDs := map[string]bool{}
		for _, n := range nodes {
			nodeIDs[n["id"].(string)] = true
		}
		for _, gn := range graph.Nodes {
			if !nodeIDs[gn.Key] {
				nodes = append(nodes, map[string]any{
					"id":          gn.Key,
					"name":        gn.Name,
					"platform":    gn.Platform,
					"classes":     0,
					"endpoints":   0,
					"stereotypes": map[string]int{},
					"external":    true,
				})
			}
		}

		edges := make([]map[string]string, 0, len(graph.Edges))
		for _, e := range graph.Edges {
			edges = append(edges, map[string]string{
				"from":     e.From,
				"to":       e.To,
				"type":     string(e.Type),
				"evidence": e.Evidence,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"nodes": nodes, "edges": edges})
	}
}

// ── Sprint Board API ─────────────────────────────────────────────────

func handleSprint(jiraClient *jira.Client, services map[string]*model.ServiceIndex) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if !jiraClient.Available() {
			json.NewEncoder(w).Encode(map[string]any{"error": "Jira not configured", "tickets": []any{}})
			return
		}

		jql := r.URL.Query().Get("jql")
		if jql == "" {
			jql = "project = PROJ AND sprint in openSprints() ORDER BY priority DESC, updated DESC"
		}

		sr, err := jiraClient.SearchJQL(jql, 200)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "tickets": []any{}})
			return
		}

		statusCounts := map[string]int{}
		typeCounts := map[string]int{}
		assigneeCounts := map[string]int{}

		tickets := make([]map[string]any, 0, len(sr.Issues))
		for _, issue := range sr.Issues {
			ticket := map[string]any{
				"key":        issue.Key,
				"summary":    issue.Summary,
				"type":       issue.Type,
				"status":     issue.Status,
				"priority":   issue.Priority,
				"assignee":   issue.Assignee,
				"components": issue.Components,
				"labels":     issue.Labels,
				"created":    issue.Created,
				"updated":    issue.Updated,
			}

			statusCounts[issue.Status]++
			typeCounts[issue.Type]++
			if issue.Assignee != "" {
				assigneeCounts[issue.Assignee]++
			} else {
				assigneeCounts["Unassigned"]++
			}

			keywords := strings.Fields(strings.ToLower(issue.Summary + " " + strings.Join(issue.Components, " ")))
			var filtered []string
			for _, kw := range keywords {
				kw = strings.Trim(kw, "|()-/")
				if len(kw) >= 4 {
					filtered = append(filtered, kw)
				}
			}
			matchedServices := []string{}
			for key := range services {
				keyLower := strings.ToLower(key)
				name := keyLower
				if i := strings.LastIndex(keyLower, ":"); i >= 0 {
					name = keyLower[i+1:]
				}
				for _, kw := range filtered {
					if strings.Contains(name, kw) {
						matchedServices = append(matchedServices, key)
						break
					}
				}
			}
			if len(matchedServices) > 5 {
				matchedServices = matchedServices[:5]
			}
			ticket["services"] = matchedServices

			tickets = append(tickets, ticket)
		}

		summary := map[string]any{
			"total":     sr.Total,
			"byStatus":  statusCounts,
			"byType":    typeCounts,
			"byAssignee": assigneeCounts,
		}

		json.NewEncoder(w).Encode(map[string]any{"tickets": tickets, "jql": jql, "summary": summary})
	}
}

// ── Single Ticket Detail API ─────────────────────────────────────

func handleTicket(jiraClient *jira.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		key := r.URL.Query().Get("key")
		if key == "" {
			json.NewEncoder(w).Encode(map[string]string{"error": "key required"})
			return
		}

		if !jiraClient.Available() {
			json.NewEncoder(w).Encode(map[string]string{"error": "Jira not configured"})
			return
		}

		issue, err := jiraClient.GetIssue(key)
		if err != nil {
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		transitions, _ := jiraClient.GetTransitions(key)

		json.NewEncoder(w).Encode(map[string]any{
			"issue":       issue,
			"transitions": transitions,
		})
	}
}

// ── MCP Event Monitor API ────────────────────────────────────────────

func handleMCPEvents(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mode := r.URL.Query().Get("mode")

		if mode == "stream" {
			streamMCPEvents(w, r, cfg)
			return
		}

		events := readRecentEvents(cfg, 50)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"events": events})
	}
}

func streamMCPEvents(w http.ResponseWriter, r *http.Request, cfg *config.Config) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 500)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	logPath := filepath.Join(cfg.StateDir(), "mcp-events.jsonl")

	lastSize := int64(0)
	if info, err := os.Stat(logPath); err == nil {
		lastSize = info.Size()
	}

	ctx := r.Context()
	notify := ctx.Done()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-notify:
			return
		case <-ticker.C:
			info, err := os.Stat(logPath)
			if err != nil || info.Size() <= lastSize {
				continue
			}

			f, err := os.Open(logPath)
			if err != nil {
				continue
			}
			f.Seek(lastSize, 0)
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := scanner.Text()
				if line != "" {
					sendSSE(w, flusher, "mcp_event", line)
				}
			}
			lastSize = info.Size()
			f.Close()
		}
	}
}

func readRecentEvents(cfg *config.Config, n int) []json.RawMessage {
	logPath := filepath.Join(cfg.StateDir(), "mcp-events.jsonl")
	f, err := os.Open(logPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var lines []json.RawMessage
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		raw := json.RawMessage(scanner.Bytes())
		lines = append(lines, raw)
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

// ── Pipeline Runner ──────────────────────────────────────────────────

func handleRun(w http.ResponseWriter, r *http.Request) {
	ticket := r.URL.Query().Get("ticket")
	if ticket == "" {
		http.Error(w, "ticket required", 400)
		return
	}
	command := r.URL.Query().Get("cmd")
	if command == "" {
		command = "plan"
	}
	streamPipeline(w, command, ticket)
}

func handleDemo(w http.ResponseWriter, r *http.Request) {
	streamPipeline(w, "plan", "PROJ-1001")
}

func handleStop(w http.ResponseWriter, r *http.Request) {
	activeProcMu.Lock()
	proc := activeProc
	activeProcMu.Unlock()

	if proc != nil {
		proc.Kill()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "killed"})
	} else {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "no active pipeline"})
	}
}

func streamPipeline(w http.ResponseWriter, command, ticket string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", 500)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	nexusPath, err := os.Executable()
	if err != nil {
		nexusPath = "nexus"
	}

	cmd := exec.Command(nexusPath, command, ticket)
	cmd.Env = os.Environ()

	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		sendSSE(w, flusher, "error_result", map[string]string{"error": err.Error()})
		return
	}

	activeProcMu.Lock()
	activeProc = cmd.Process
	activeProcMu.Unlock()

	defer func() {
		activeProcMu.Lock()
		activeProc = nil
		activeProcMu.Unlock()
	}()

	currentPhase := ""
	prevPhase := ""
	phaseStart := time.Now()

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				lines := strings.Split(string(buf[:n]), "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					sendSSE(w, flusher, "log", line)

					phase := extractPhase(line)
					if phase != "" && phase != currentPhase {
						prevPhase = currentPhase
						elapsed := time.Since(phaseStart).Round(time.Millisecond).String()
						phaseStart = time.Now()
						currentPhase = phase
						sendSSE(w, flusher, "phase", map[string]string{
							"name":      phase,
							"prev":      prevPhase,
							"prev_time": elapsed,
						})
					}
				}
			}
			if err != nil {
				break
			}
		}
	}()

	var resultBuf strings.Builder
	go func() {
		io.Copy(&resultBuf, stdout)
	}()

	err = cmd.Wait()
	time.Sleep(100 * time.Millisecond)

	if err != nil {
		sendSSE(w, flusher, "error_result", map[string]string{
			"error": err.Error(),
			"phase": currentPhase,
		})
		return
	}

	result := map[string]any{
		"phase": currentPhase,
		"time":  time.Since(phaseStart).Round(time.Millisecond).String(),
	}

	var parsed map[string]any
	if json.Unmarshal([]byte(resultBuf.String()), &parsed) == nil {
		for k, v := range parsed {
			result[k] = v
		}
	}

	sendSSE(w, flusher, "done", result)
}

func sendSSE(w http.ResponseWriter, f http.Flusher, event string, data any) {
	var payload string
	switch v := data.(type) {
	case string:
		payload = v
	default:
		b, _ := json.Marshal(v)
		payload = string(b)
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload)
	f.Flush()
}

func extractPhase(line string) string {
	phases := []string{"CLASSIFY", "INVESTIGATE", "CONTEXT"}
	for _, p := range phases {
		if strings.Contains(line, "["+p+"]") {
			return p
		}
	}
	return ""
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		cmd = exec.Command("open", url)
	}
	cmd.Run()
}
