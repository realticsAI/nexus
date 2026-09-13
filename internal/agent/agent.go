package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/anurag/nexus/internal/config"
	"github.com/anurag/nexus/internal/confluence"
	"github.com/anurag/nexus/internal/debug"
	"github.com/anurag/nexus/internal/figma"
	"github.com/anurag/nexus/internal/gitops"
	"github.com/anurag/nexus/internal/jira"
	"github.com/anurag/nexus/internal/llm"
	"github.com/anurag/nexus/internal/security"
	"github.com/anurag/nexus/internal/store"
	"github.com/anurag/nexus/model"
)

type Agent struct {
	jira        *jira.Client
	confluence  *confluence.Client
	figma       *figma.Client
	llm         llm.Provider
	store       *store.Store
	cfg         config.AgentConfig
	dryRun      bool
	contextOnly bool
	validate    bool
	generateTD  bool
	forcePlan   bool
	audit          *security.AuditLogger
	summaries      map[string]*model.ServiceSummary
	loadedServices map[string]*model.ServiceIndex
	graph          *model.Graph
}

func New(jiraClient *jira.Client, confClient *confluence.Client, figmaClient *figma.Client, llmProvider llm.Provider, s *store.Store, cfg config.AgentConfig, dryRun bool) *Agent {
	return &Agent{
		jira:       jiraClient,
		confluence: confClient,
		figma:      figmaClient,
		llm:        llmProvider,
		store:      s,
		cfg:        cfg,
		dryRun:     dryRun,
		audit:      security.NewAuditLogger(filepath.Dir(s.Dir())),
	}
}

func (a *Agent) SetAudit(audit *security.AuditLogger) { a.audit = audit }

func (a *Agent) SetContextOnly(v bool) { a.contextOnly = v }
func (a *Agent) SetValidate(v bool)    { a.validate = v }
func (a *Agent) SetGenerateTD(v bool)  { a.generateTD = v }
func (a *Agent) SetForcePlan(v bool)   { a.forcePlan = v }

func (a *Agent) loadService(key string) (*model.ServiceIndex, error) {
	if svc, ok := a.loadedServices[key]; ok {
		debug.Log("agent", "loadService key=%s (cached, units=%d)", key, len(svc.Units))
		return svc, nil
	}
	memBefore := debug.MemAlloc()
	svc, err := a.store.LoadService(key)
	if err != nil {
		return nil, err
	}
	a.loadedServices[key] = svc
	memAfter := debug.MemAlloc()
	delta := "n/a"
	if memAfter >= memBefore {
		delta = "+" + debug.FormatBytes(memAfter-memBefore)
	} else {
		delta = "-" + debug.FormatBytes(memBefore-memAfter) + " (GC)"
	}
	debug.Log("agent", "loadService key=%s units=%d memDelta=%s totalMem=%s", key, len(svc.Units), delta, debug.FormatBytes(memAfter))
	return svc, nil
}

func log(phase WorkPhase, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "[%s] %s\n", phase, msg)
}

func logStep(phase WorkPhase, step, detail string) {
	fmt.Fprintf(os.Stderr, "[%s]   -> %s: %s\n", phase, step, detail)
}

func memMB() string {
	if !debug.Enabled() {
		return ""
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return fmt.Sprintf("alloc=%s sys=%s", debug.FormatBytes(m.Alloc), debug.FormatBytes(m.Sys))
}

func (a *Agent) Work(ticketKey string) (*WorkResult, error) {
	totalStart := time.Now()
	var phaseStart time.Time
	result := &WorkResult{TicketKey: ticketKey, DryRun: a.dryRun}
	debug.Log("agent", "Work started ticket=%s contextOnly=%v dryRun=%v %s", ticketKey, a.contextOnly, a.dryRun, memMB())

	// Load lightweight summaries (~1KB each) instead of full indexes (5-50MB each)
	log("INIT", "Loading service summaries...")
	var err error
	a.summaries, err = a.store.LoadServiceSummaries()
	if err != nil || len(a.summaries) == 0 {
		a.summaries = map[string]*model.ServiceSummary{}
		log("INIT", "No summaries found, backfilling...")
		debug.Log("agent", "No summaries, attempting backfill %s", memMB())
		if n, bErr := a.store.BackfillSummaries(); bErr == nil && n > 0 {
			log("INIT", "Backfilled %d summaries", n)
			a.summaries, _ = a.store.LoadServiceSummaries()
		}
	}
	debug.Log("agent", "Summaries loaded count=%d %s", len(a.summaries), memMB())
	a.loadedServices = make(map[string]*model.ServiceIndex)
	if len(a.summaries) == 0 {
		log("INIT", "No summaries available, falling back to full service load...")
		debug.Log("agent", "FALLBACK to LoadAllServices — this is the OOM danger path")
		all, loadErr := a.store.LoadAllServices()
		if loadErr != nil {
			return nil, fmt.Errorf("loading services: %w", loadErr)
		}
		for key, svc := range all {
			a.loadedServices[key] = svc
			a.summaries[key] = model.BuildServiceSummary(svc)
		}
		debug.Log("agent", "FALLBACK complete services=%d %s", len(a.loadedServices), memMB())
	}
	a.graph, err = a.store.LoadGraph()
	if err != nil {
		a.graph = &model.Graph{}
	}
	log("INIT", "Loaded %d service summaries, %d graph edges", len(a.summaries), len(a.graph.Edges))
	debug.Log("agent", "INIT complete summaries=%d graph_edges=%d %s", len(a.summaries), len(a.graph.Edges), memMB())

	// ── Pipeline Tree: load cached run and compute fingerprints ──────
	stateDir := a.store.Dir()
	tree, treeErr := LoadPipelineTree(stateDir, ticketKey)
	if treeErr != nil {
		debug.Log("agent", "LoadPipelineTree error (starting fresh): %v", treeErr)
		tree = nil
	}

	// Always fetch fresh ticket to compute fingerprint
	issue, err := a.jira.GetIssue(ticketKey)
	if err != nil {
		result.Error = err.Error()
		return result, fmt.Errorf("reading ticket: %w", err)
	}

	ticketHash := FingerprintTicket(issue)
	indexHash := FingerprintIndex(a.summaries, a.graph)
	llmHash := FingerprintLLM(a.llm)

	var resumeFrom WorkPhase
	if tree != nil && !a.dryRun {
		tree.CheckAndInvalidate(ticketHash, indexHash, llmHash)
		resumeFrom = tree.ResumePoint()
		valid, invalid := tree.PhaseCount()
		if resumeFrom != PhaseUnderstand {
			log("INIT", "Cached run found (run %s) — %d valid, %d invalid phases. Resuming from %s",
				tree.RunID, valid, invalid, resumeFrom)
		}
	} else {
		tree = NewPipelineTree(ticketKey)
		resumeFrom = PhaseUnderstand
	}

	// Helper: should we run this phase or use cached data?
	shouldRun := func(phase WorkPhase) bool {
		for i, p := range PhaseOrder {
			if p == resumeFrom {
				for j, q := range PhaseOrder {
					if q == phase {
						return j >= i
					}
				}
			}
		}
		return true
	}

	saveTree := func() {
		if sErr := SavePipelineTree(stateDir, ticketKey, tree); sErr != nil {
			debug.Log("agent", "SavePipelineTree error: %v", sErr)
		}
	}

	// ── Phase 1: UNDERSTAND ──────────────────────────────────────────
	var ticketType TicketType
	if shouldRun(PhaseUnderstand) {
		result.Phase = PhaseUnderstand
		phaseStart = time.Now()
		log(PhaseUnderstand, "Fetching Jira ticket %s...", ticketKey)

		// Prompt injection scan on ticket content
		injCheck := security.ScanForInjection(issue.Description + " " + issue.AccCriteria)
		if !injCheck.Safe {
			log(PhaseUnderstand, "SECURITY: prompt injection detected (severity: %s)", injCheck.Severity)
			for _, t := range injCheck.Threats {
				logStep(PhaseUnderstand, "Threat", fmt.Sprintf("[%s] %s: %s", t.Severity, t.Type, t.Pattern))
			}
			if injCheck.Severity == "high" {
				a.audit.LogJira("injection_blocked", ticketKey, fmt.Sprintf("severity=%s threats=%d", injCheck.Severity, len(injCheck.Threats)), nil)
				result.Error = "ticket blocked: prompt injection detected"
				return result, fmt.Errorf("prompt injection detected in ticket %s (severity: %s) — review ticket description manually", ticketKey, injCheck.Severity)
			}
		}

		ticketType = Classify(issue)
		result.TicketType = ticketType
		log(PhaseUnderstand, "Ticket: %s", issue.Summary)
		logStep(PhaseUnderstand, "Type", string(ticketType))
		logStep(PhaseUnderstand, "Components", strings.Join(issue.Components, ", "))
		logStep(PhaseUnderstand, "Labels", strings.Join(issue.Labels, ", "))
		logStep(PhaseUnderstand, "Description", fmt.Sprintf("%d chars", len(issue.Description)))
		logStep(PhaseUnderstand, "Acceptance Criteria", fmt.Sprintf("%d chars", len(issue.AccCriteria)))
		if issue.DueDate != "" {
			logStep(PhaseUnderstand, "Due date", issue.DueDate)
		}
		if len(issue.LinkedIssues) > 0 {
			logStep(PhaseUnderstand, "Linked issues", strings.Join(issue.LinkedIssues, ", "))
		}
		logStep(PhaseUnderstand, "Duration", time.Since(phaseStart).Round(time.Millisecond).String())

		tree.Issue = issue
		tree.TicketType = ticketType
		tree.AddNode(PhaseUnderstand, ticketHash, time.Since(phaseStart), 0)
		saveTree()
	} else {
		issue = tree.Issue
		ticketType = tree.TicketType
		result.TicketType = ticketType
		log(PhaseUnderstand, "Using cached result (ticket unchanged)")
	}

	// ── Phase 2: INVESTIGATE ─────────────────────────────────────────
	var investigation *InvestigationContext
	if shouldRun(PhaseInvestigate) {
	result.Phase = PhaseInvestigate
	phaseStart = time.Now()
	log(PhaseInvestigate, "Scanning codebase for relevant code...")

	// Targeted loading: resolve relevant services from summaries, load only those
	debug.Log("agent", "ResolveServiceKeys starting %s", memMB())
	matchedKeys := ResolveServiceKeys(issue, a.summaries)
	debug.Log("agent", "ResolveServiceKeys matched=%d keys=%v", len(matchedKeys), matchedKeys)
	if platform := DetectTicketPlatform(issue); platform != PlatformUnknown && platform != PlatformBackend {
		if pKey, found := FindPlatformRepoFromSummaries(a.summaries, platform); found {
			has := false
			for _, k := range matchedKeys {
				if k == pKey {
					has = true
					break
				}
			}
			if !has {
				matchedKeys = append(matchedKeys, pKey)
			}
		}
	}
	if len(matchedKeys) > 5 {
		matchedKeys = matchedKeys[:5]
	}
	log(PhaseInvestigate, "Summary matching: %d/%d services relevant", len(matchedKeys), len(a.summaries))
	for _, key := range matchedKeys {
		if _, ok := a.loadedServices[key]; ok {
			continue
		}
		svc, loadErr := a.loadService(key)
		if loadErr != nil {
			log(PhaseInvestigate, "Warning: failed to load %s: %v", key, loadErr)
			continue
		}
		logStep(PhaseInvestigate, "Loaded", fmt.Sprintf("%s (%d units)", key, len(svc.Units)))
	}

	var allServiceKeys []string
	for key := range a.summaries {
		allServiceKeys = append(allServiceKeys, key)
	}

	githubOrg := ""
	if len(a.cfg.GitHubOrg) > 0 {
		githubOrg = a.cfg.GitHubOrg
	}
	investigation = Investigate(issue, a.loadedServices, a.graph, githubOrg, allServiceKeys)

	// Platform awareness
	if investigation.TicketPlatform != "" {
		logStep(PhaseInvestigate, "Platform", string(investigation.TicketPlatform))
	}
	for _, w := range investigation.Warnings {
		log(PhaseInvestigate, "⚠ %s", w)
	}
	if len(investigation.BackendAPIs) > 0 {
		logStep(PhaseInvestigate, "Backend APIs for app", fmt.Sprintf("%d endpoints the %s app can consume", len(investigation.BackendAPIs), investigation.TicketPlatform))
	}

	logStep(PhaseInvestigate, "Matched services", fmt.Sprintf("%d — primary: %s", len(investigation.Services), safeFirst(investigation.Services)))
	logStep(PhaseInvestigate, "Related classes", fmt.Sprintf("%d found", len(investigation.RelatedClasses)))
	logStep(PhaseInvestigate, "Call chain", fmt.Sprintf("%d links traced", len(investigation.CallChain)))
	logStep(PhaseInvestigate, "Existing endpoints", fmt.Sprintf("%d in primary module", len(investigation.ExistingEndpoints)))
	logStep(PhaseInvestigate, "External APIs", fmt.Sprintf("%d facades detected", len(investigation.ExternalAPIs)))
	logStep(PhaseInvestigate, "File signatures", fmt.Sprintf("%d read from disk", len(investigation.FileExcerpts)))
	logStep(PhaseInvestigate, "Patterns", strings.Join(investigation.Patterns, ", "))
	logStep(PhaseInvestigate, "Dependencies", strings.Join(investigation.Dependencies, ", "))

	// Figma
	if a.figma != nil && a.figma.Available() {
		figmaLinks := figma.ExtractFigmaLinks(issue.AllText())
		log(PhaseInvestigate, "Scanning for Figma links... found %d in ticket", len(figmaLinks))

		if len(figmaLinks) == 0 && len(issue.LinkedIssues) > 0 {
			log(PhaseInvestigate, "No Figma in main ticket — scanning %d linked issues...", len(issue.LinkedIssues))
			for _, linkedKey := range issue.LinkedIssues {
				linkedIssue, err := a.jira.GetIssue(linkedKey)
				if err != nil {
					logStep(PhaseInvestigate, linkedKey, "fetch failed, skipping")
					continue
				}
				found := figma.ExtractFigmaLinks(linkedIssue.AllText())
				if len(found) > 0 {
					logStep(PhaseInvestigate, linkedKey, fmt.Sprintf("found %d Figma links", len(found)))
				}
				figmaLinks = append(figmaLinks, found...)
			}
		}

		for _, link := range figmaLinks {
			fileKey, nodeID := figma.ParseFigmaURL(link)
			if fileKey == "" {
				continue
			}
			if nodeID != "" {
				log(PhaseInvestigate, "Fetching Figma node %s (file=%s)...", nodeID, fileKey)
			} else {
				log(PhaseInvestigate, "Fetching Figma file %s...", fileKey)
			}

			ref := FigmaDesignRef{URL: link, FileKey: fileKey, NodeID: nodeID}

			if nodeID != "" {
				nodes, err := a.figma.GetFileNodes(fileKey, []string{nodeID})
				if err != nil {
					logStep(PhaseInvestigate, "Figma", fmt.Sprintf("node fetch failed: %v", err))
					continue
				}
				if node, ok := nodes[nodeID]; ok && node != nil {
					ref.FileName = node.Name
					ref.UIText = figma.ExtractAllText(node, 10)
					ref.ScreenStates = figma.ExtractFrameNames(node)
					ref.Frames = ref.ScreenStates
					ref.ScreenAnalysis = figma.AnalyzeAllScreens(node)
					screenCount := 0
					edgeCaseCount := 0
					if ref.ScreenAnalysis != nil {
						screenCount = len(ref.ScreenAnalysis.Screens)
						for _, s := range ref.ScreenAnalysis.Screens {
							edgeCaseCount += len(s.EdgeCases)
						}
					}
					logStep(PhaseInvestigate, "Figma", fmt.Sprintf("got node '%s': %d screens analyzed, %d edge cases, %d backend needs, %d frontend logic",
						ref.FileName, screenCount, edgeCaseCount,
						len(ref.ScreenAnalysis.BackendNeeds), len(ref.ScreenAnalysis.FrontendLogic)))
				}
			} else {
				file, err := a.figma.GetFile(fileKey)
				if err != nil {
					logStep(PhaseInvestigate, "Figma", fmt.Sprintf("file fetch failed: %v", err))
					continue
				}
				ref.FileName = file.Name
				if file.Document != nil {
					for _, page := range file.Document.Children {
						if page.Type != "CANVAS" {
							continue
						}
						ref.Pages = append(ref.Pages, page.Name)
						for _, frame := range page.Children {
							if frame.Type == "FRAME" || frame.Type == "COMPONENT" || frame.Type == "COMPONENT_SET" {
								ref.Frames = append(ref.Frames, frame.Name)
								ref.UIText = append(ref.UIText, figma.ExtractAllText(frame, 3)...)
							}
						}
					}
					for id, comp := range file.Components {
						_ = id
						ref.Components = append(ref.Components, comp.Name)
					}
					ref.ScreenAnalysis = figma.AnalyzeAllScreens(file.Document)
				}
				screenCount := 0
				if ref.ScreenAnalysis != nil {
					screenCount = len(ref.ScreenAnalysis.Screens)
				}
				logStep(PhaseInvestigate, "Figma", fmt.Sprintf("got '%s': %d pages, %d components, %d frames, %d screens analyzed", ref.FileName, len(ref.Pages), len(ref.Components), len(ref.Frames), screenCount))
			}

			investigation.FigmaDesigns = append(investigation.FigmaDesigns, ref)
		}
	}

	// Linked issues
	if len(issue.LinkedIssues) > 0 {
		log(PhaseInvestigate, "Fetching %d linked issues...", len(issue.LinkedIssues))
		for _, linkedKey := range issue.LinkedIssues {
			linkedIssue, err := a.jira.GetIssue(linkedKey)
			if err != nil {
				logStep(PhaseInvestigate, linkedKey, "fetch failed, skipping")
				continue
			}
			investigation.LinkedIssues = append(investigation.LinkedIssues, LinkedIssueSummary{
				Key:      linkedIssue.Key,
				Summary:  linkedIssue.Summary,
				Status:   linkedIssue.Status,
				Type:     linkedIssue.Type,
				Assignee: linkedIssue.Assignee,
			})
			logStep(PhaseInvestigate, linkedKey, fmt.Sprintf("[%s] %s — %s", linkedIssue.Status, linkedIssue.Summary, linkedIssue.Assignee))
		}
	}

	logStep(PhaseInvestigate, "Duration", time.Since(phaseStart).Round(time.Millisecond).String())
	log(PhaseInvestigate, "Summary: %s", investigation.Summary())

	tree.Investigation = investigation
	tree.AddNode(PhaseInvestigate, indexHash, time.Since(phaseStart), 0)
	saveTree()

	if a.contextOnly {
		result.Investigation = investigation
		log("DONE", "Context-only mode — returning investigation JSON (%s total)", time.Since(totalStart).Round(time.Millisecond))
		return result, nil
	}
	} else {
		investigation = tree.Investigation
		log(PhaseInvestigate, "Using cached result (%d services, %d classes)", len(investigation.Services), len(investigation.RelatedClasses))
	}

	// ── Service validation: ask LLM to confirm which service owns the behavior ──
	if len(investigation.Services) >= 2 {
		log(PhaseInvestigate, "Validating service selection with LLM (%d candidates)...", len(investigation.Services))
		validated, vTokens, vErr := ValidateServiceSelection(a.llm, issue, investigation.Services, a.summaries)
		if vErr != nil {
			log(PhaseInvestigate, "Service validation failed (using keyword order): %v", vErr)
		} else {
			result.TokensUsed += vTokens
			overridden := validated[0] != investigation.Services[0]
			if overridden {
				log(PhaseInvestigate, "⚠ LLM OVERRIDE: %s → %s (keyword scoring picked wrong service)", investigation.Services[0], validated[0])
			} else {
				log(PhaseInvestigate, "LLM confirmed primary service: %s", validated[0])
			}
			investigation.Services = validated

			if overridden {
				log(PhaseInvestigate, "Refreshing context for new primary: %s", validated[0])
				allServices, _ := a.store.LoadAllServices()
				RefreshPrimaryContext(investigation, allServices, a.cfg.GitHubOrg)
				log(PhaseInvestigate, "Context refreshed: %d excerpts, %d endpoints, %d existing files",
					len(investigation.FileExcerpts), len(investigation.ExistingEndpoints), len(investigation.ExistingFiles))
			}
		}
	}

	// Resolve repo paths for all matched services
	if len(investigation.Services) > 0 {
		investigation.ServicePaths = make(map[string]string)
		for _, svcKey := range investigation.Services {
			if svc, err := a.loadService(svcKey); err == nil {
				investigation.ServicePaths[svcKey] = svc.Path
			}
		}
		if investigation.RepoPath == "" {
			if path, ok := investigation.ServicePaths[investigation.Services[0]]; ok {
				investigation.RepoPath = path
			}
		}
	}

	// ── Phase 2b: ANALYZE ────────────────────────────────────────────
	var analysis *AnalysisResult
	if shouldRun(PhaseAnalyze) {
	result.Phase = PhaseAnalyze
	phaseStart = time.Now()
	log(PhaseAnalyze, "Calling LLM (%s) to analyze requirements...", a.llm.Name())

	analyzeStart := time.Now()
	var aTokens int
	analysis, aTokens, err = AnalyzeTicket(a.llm, issue, ticketType, investigation)
	a.audit.LogLLM(ticketKey, a.llm.Model(), aTokens, time.Since(analyzeStart), err)
	if err != nil {
		log(PhaseAnalyze, "Warning: analysis failed: %v — proceeding without requirements", err)
	} else {
		result.TokensUsed += aTokens
		result.Analysis = analysis
		log(PhaseAnalyze, "LLM responded: %d tokens used", aTokens)
		logStep(PhaseAnalyze, "Requirements", fmt.Sprintf("%d extracted", len(analysis.Requirements)))

		musts, shoulds := 0, 0
		for _, r := range analysis.Requirements {
			switch r.Priority {
			case "must":
				musts++
			case "should":
				shoulds++
			}
		}
		logStep(PhaseAnalyze, "Priority", fmt.Sprintf("%d must, %d should, %d nice", musts, shoulds, len(analysis.Requirements)-musts-shoulds))

		cats := make(map[string]int)
		for _, r := range analysis.Requirements {
			cats[r.Category]++
		}
		for cat, cnt := range cats {
			logStep(PhaseAnalyze, cat, fmt.Sprintf("%d requirements", cnt))
		}

		logStep(PhaseAnalyze, "Edge cases", fmt.Sprintf("%d identified", len(analysis.EdgeCases)))
		for i, ec := range analysis.EdgeCases {
			logStep(PhaseAnalyze, fmt.Sprintf("EC-%d", i+1), ec.Trigger)
		}

		if len(analysis.Unknowns) > 0 {
			logStep(PhaseAnalyze, "Unknowns", fmt.Sprintf("%d questions", len(analysis.Unknowns)))
			for _, u := range analysis.Unknowns {
				logStep(PhaseAnalyze, "?", u)
			}
		}
	}
	logStep(PhaseAnalyze, "Duration", time.Since(phaseStart).Round(time.Millisecond).String())

	tree.Analysis = analysis
	tree.AddNode(PhaseAnalyze, llmHash, time.Since(phaseStart), aTokens)
	saveTree()
	} else {
		analysis = tree.Analysis
		if analysis != nil {
			result.Analysis = analysis
			log(PhaseAnalyze, "Using cached result (%d requirements, %d edge cases)", len(analysis.Requirements), len(analysis.EdgeCases))
		} else {
			log(PhaseAnalyze, "Using cached result (no analysis)")
		}
	}

	// ── Prep ticket short-circuit (only in auto mode, not explicit plan/handoff) ──
	if ticketType == Prep && !a.forcePlan {
		log(PhasePlan, "PREP ticket detected — generating investigation report instead of code plan")
		log(PhasePlan, "Skipping code plan + validate (saves ~100K tokens)")
		log(PhasePlan, "To force full analysis on a prep ticket, use 'nexus plan' or 'nexus handoff' directly")

		prepPlan := &Plan{
			Summary: fmt.Sprintf("Investigation report for prep ticket %s", ticketKey),
			Steps:   buildPrepSteps(investigation, analysis),
		}
		result.Plan = prepPlan
		result.Investigation = investigation

		if a.dryRun {
			log("DRY_RUN", "Would produce investigation report with %d items (no code changes)", len(prepPlan.Steps))
			log("DONE", "Dry run complete — total time %s, %d tokens", time.Since(totalStart).Round(time.Millisecond), result.TokensUsed)
			return result, nil
		}

		result.Phase = PhasePlan
		logStep(PhasePlan, "Report items", fmt.Sprintf("%d", len(prepPlan.Steps)))
		logStep(PhasePlan, "Duration", time.Since(phaseStart).Round(time.Millisecond).String())
		log("DONE", "Prep investigation complete — total time %s, %d tokens", time.Since(totalStart).Round(time.Millisecond), result.TokensUsed)
		return result, nil
	}

	if ticketType == Prep {
		log(PhasePlan, "⚠ PREP ticket — running full pipeline as explicitly requested. This will use ~100K tokens.")
	}

	// ── Phase 3: PLAN ────────────────────────────────────────────────
	var plan *Plan
	if shouldRun(PhasePlan) {
		result.Phase = PhasePlan
		phaseStart = time.Now()
		log(PhasePlan, "Calling LLM (%s) to generate implementation plan...", a.llm.Name())

		planStart := time.Now()
		var pTokens int
		plan, pTokens, err = BuildPlan(a.llm, issue, ticketType, investigation, analysis)
		a.audit.LogLLM(ticketKey, a.llm.Model(), pTokens, time.Since(planStart), err)
		if err != nil {
			result.Error = err.Error()
			return result, err
		}
		result.TokensUsed += pTokens
		log(PhasePlan, "LLM responded: %d tokens used", pTokens)
		logStep(PhasePlan, "Plan summary", plan.Summary)
		logStep(PhasePlan, "Steps", fmt.Sprintf("%d total", len(plan.Steps)))
		creates, edits := 0, 0
		for _, s := range plan.Steps {
			if s.Action == "create" {
				creates++
			} else {
				edits++
			}
		}
		logStep(PhasePlan, "Breakdown", fmt.Sprintf("%d creates, %d edits", creates, edits))
		for i, step := range plan.Steps {
			covers := ""
			if len(step.Covers) > 0 {
				covers = fmt.Sprintf(" [covers: %s]", strings.Join(step.Covers, ", "))
			}
			logStep(PhasePlan, fmt.Sprintf("Step %d", i+1), fmt.Sprintf("%s %s%s", step.Action, step.File, covers))
		}

		if analysis != nil && len(analysis.Requirements) > 0 {
			covered := make(map[string]bool)
			for _, step := range plan.Steps {
				for _, id := range step.Covers {
					covered[id] = true
				}
			}
			uncovered := 0
			for _, r := range analysis.Requirements {
				if !covered[r.ID] {
					uncovered++
					logStep(PhasePlan, "UNCOVERED", fmt.Sprintf("%s: %s", r.ID, r.Description))
				}
			}
			if uncovered == 0 {
				logStep(PhasePlan, "Coverage", fmt.Sprintf("all %d requirements covered", len(analysis.Requirements)))
			} else {
				logStep(PhasePlan, "Coverage", fmt.Sprintf("%d/%d covered, %d MISSING", len(analysis.Requirements)-uncovered, len(analysis.Requirements), uncovered))
			}
		}
		logStep(PhasePlan, "Duration", time.Since(phaseStart).Round(time.Millisecond).String())

		TagPlanStepsWithService(plan.Steps, investigation)
		tree.Plan = plan
		tree.AddNode(PhasePlan, llmHash, time.Since(phaseStart), pTokens)
		saveTree()
	} else {
		plan = tree.Plan
		if plan != nil {
			log(PhasePlan, "Using cached plan (%d steps)", len(plan.Steps))
		}
	}

	// ── Scope budget check (pre-validate) ───────────────────────────
	if plan != nil {
		uniqueFiles := map[string]bool{}
		for _, step := range plan.Steps {
			uniqueFiles[step.File] = true
		}
		fileList := make([]string, 0, len(uniqueFiles))
		for f := range uniqueFiles {
			fileList = append(fileList, f)
		}
		scopeViolations := gitops.DefaultSafety().ValidateScope(string(ticketType), len(plan.Steps), fileList)
		if len(scopeViolations) > 0 {
			for _, v := range scopeViolations {
				log(PhasePlan, "SCOPE WARNING: %s", v.Message)
			}
			if ticketType == Bug && !a.dryRun {
				result.Error = fmt.Sprintf("scope budget exceeded for BUG ticket: %s", scopeViolations[0].Message)
				return result, fmt.Errorf("plan scope too large for BUG ticket — reduce file count or reclassify")
			}
		}
	}

	// ── Phase 3b: VALIDATE ───────────────────────────────────────────
	if a.validate || a.dryRun {
		if shouldRun(PhaseValidate) {
			result.Phase = PhaseValidate
			phaseStart = time.Now()
			log(PhaseValidate, "Calling LLM (%s) to validate plan against codebase...", a.llm.Name())

			validateStart := time.Now()
			validation, vTokens, vErr := ValidatePlan(a.llm, issue, ticketType, investigation, plan)
			a.audit.LogLLM(ticketKey, a.llm.Model(), vTokens, time.Since(validateStart), vErr)
			if vErr != nil {
				log(PhaseValidate, "Warning: validation LLM call failed: %v", vErr)
			} else {
				result.TokensUsed += vTokens
				result.Validation = validation
				log(PhaseValidate, "LLM responded: %d tokens used", vTokens)
				logStep(PhaseValidate, "Verdict", validation.Verdict)

				for _, d := range validation.Dimensions {
					logStep(PhaseValidate, d.Name, fmt.Sprintf("[%s] %s", d.Verdict, d.Detail))
				}
				if len(validation.Risks) > 0 {
					log(PhaseValidate, "Risks identified: %d", len(validation.Risks))
					for _, r := range validation.Risks {
						logStep(PhaseValidate, "Risk", r)
					}
				}
				if len(validation.Suggestions) > 0 {
					log(PhaseValidate, "Suggestions: %d", len(validation.Suggestions))
					for _, s := range validation.Suggestions {
						logStep(PhaseValidate, "Suggest", s)
					}
				}
				if len(validation.MissingFromPlan) > 0 {
					log(PhaseValidate, "Missing from plan: %d items", len(validation.MissingFromPlan))
					for _, m := range validation.MissingFromPlan {
						logStep(PhaseValidate, "Missing", m)
					}
				}
			}
			logStep(PhaseValidate, "Duration", time.Since(phaseStart).Round(time.Millisecond).String())

			// Enforce verdict — FAIL blocks pipeline
			if validation != nil && validation.Verdict == "FAIL" && !a.dryRun {
				failDimensions := []string{}
				for _, d := range validation.Dimensions {
					if d.Verdict == "FAIL" {
						failDimensions = append(failDimensions, d.Name)
					}
				}
				log(PhaseValidate, "BLOCKED: validation verdict is FAIL on: %s", strings.Join(failDimensions, ", "))
				result.Error = fmt.Sprintf("validation FAIL: %s", strings.Join(failDimensions, ", "))
				return result, fmt.Errorf("plan validation failed: %s", strings.Join(failDimensions, ", "))
			}

			tree.Validation = result.Validation
			tree.AddNode(PhaseValidate, llmHash, time.Since(phaseStart), vTokens)
			saveTree()

			// Validation gaps are advisory — included in handoff output for the agent
			// to consider, but the plan is NOT modified. REPLAN was removed because it
			// over-scoped by adding steps for gaps already satisfied by design.
			if validation != nil && len(validation.MissingFromPlan) > 0 {
				log(PhaseValidate, "Advisory: %d gaps noted (included in handoff, plan unchanged)", len(validation.MissingFromPlan))
			}
		} else {
			result.Validation = tree.Validation
			if tree.Validation != nil {
				log(PhaseValidate, "Using cached result (verdict: %s)", tree.Validation.Verdict)
			}
		}
	}

	result.Investigation = investigation
	result.Plan = plan

	// ── Phase 3c: DESIGN (TD generation) ─────────────────────────────
	if a.generateTD {
		result.Phase = PhaseDesign
		phaseStart = time.Now()
		log(PhaseDesign, "Calling LLM (%s) to generate Technical Design...", a.llm.Name())

		td, tdTokens, err := GenerateTD(a.llm, issue, ticketType, investigation, analysis, plan, result.Validation)
		if err != nil {
			log(PhaseDesign, "Warning: TD generation failed: %v", err)
		} else {
			result.TokensUsed += tdTokens
			result.TechnicalDesign = td
			lines := strings.Count(td, "\n")
			log(PhaseDesign, "LLM responded: %d tokens, %d lines generated", tdTokens, lines)
		}
		logStep(PhaseDesign, "Duration", time.Since(phaseStart).Round(time.Millisecond).String())

		log("DONE", "TD generated — total time %s, %d tokens", time.Since(totalStart).Round(time.Millisecond), result.TokensUsed)
		return result, nil
	}

	log("DONE", "Pipeline complete — total time %s, %d tokens", time.Since(totalStart).Round(time.Millisecond), result.TokensUsed)
	return result, nil
}


// enforceReplanScope trims REPLAN output to stay within budget and within the target service.
// Steps targeting files outside the primary service's package are dropped.
// If still over budget, lowest-priority steps (last added by REPLAN) are dropped.
func enforceReplanScope(steps []PlanStep, ticketType string, investigation *InvestigationContext, logFn func(string, ...interface{})) []PlanStep {
	if investigation == nil || len(investigation.Services) == 0 {
		return steps
	}

	// Detect the primary service's Java package prefix from investigation
	// e.g., "src/main/java/com/example/platform/contentservices/" for app-hub
	var allowedPrefixes []string
	for _, excerpt := range investigation.FileExcerpts {
		if excerpt.File == "" {
			continue
		}
		// Extract the package-level prefix: everything up to and including the 4th directory under src/main/java/
		// e.g., "src/main/java/com/example/platform/contentservices/" from
		// "src/main/java/com/example/platform/contentservices/appcore/services/NotificationService.java"
		prefix := extractJavaPackagePrefix(excerpt.File)
		if prefix == "" {
			continue
		}
		found := false
		for _, p := range allowedPrefixes {
			if p == prefix {
				found = true
				break
			}
		}
		if !found {
			allowedPrefixes = append(allowedPrefixes, prefix)
		}
	}
	// Also allow src/main/resources and src/test counterparts
	var extraPrefixes []string
	for _, p := range allowedPrefixes {
		testVariant := strings.Replace(p, "/main/java/", "/test/java/", 1)
		if testVariant != p {
			extraPrefixes = append(extraPrefixes, testVariant)
		}
	}
	allowedPrefixes = append(allowedPrefixes, "src/main/resources", "src/test/resources")
	allowedPrefixes = append(allowedPrefixes, extraPrefixes...)

	// Filter: only keep steps whose files are within allowed prefixes (or have no src/ prefix)
	var filtered []PlanStep
	for _, step := range steps {
		if !strings.Contains(step.File, "/src/") {
			filtered = append(filtered, step)
			continue
		}
		allowed := false
		for _, prefix := range allowedPrefixes {
			if strings.HasPrefix(step.File, prefix) {
				allowed = true
				break
			}
		}
		if allowed {
			filtered = append(filtered, step)
		} else {
			logFn("DROPPED step (outside target service): %s %s", step.Action, step.File)
		}
	}

	// Enforce file budget — count unique files
	budget, ok := gitops.GetScopeBudget(ticketType)
	if !ok {
		return filtered
	}

	uniqueFiles := map[string]bool{}
	for _, s := range filtered {
		uniqueFiles[s.File] = true
	}

	if len(uniqueFiles) <= budget.MaxFiles && len(filtered) <= budget.MaxSteps {
		return filtered
	}

	// Over budget — keep steps in order (original plan first, REPLAN additions last)
	// and drop from the end until within budget
	var result []PlanStep
	seenFiles := map[string]bool{}
	for _, s := range filtered {
		seenFiles[s.File] = true
		if len(seenFiles) > budget.MaxFiles {
			logFn("DROPPED step (file budget %d exceeded): %s %s", budget.MaxFiles, s.Action, s.File)
			continue
		}
		if len(result) >= budget.MaxSteps {
			logFn("DROPPED step (step budget %d exceeded): %s %s", budget.MaxSteps, s.Action, s.File)
			continue
		}
		result = append(result, s)
	}

	return result
}

// extractJavaPackagePrefix extracts a package-level prefix from a Java file path.
// e.g., "src/main/java/com/example/platform/contentservices/" from
// "src/main/java/com/example/platform/contentservices/appcore/services/NotificationService.java"
// This is the level that distinguishes different modules in a monorepo
// (e.g., com.example.platform.contentservices vs com.example.platform.notifications.manager).
func extractJavaPackagePrefix(filePath string) string {
	// Find "src/main/java/" or "src/test/java/" marker
	markers := []string{"src/main/java/", "src/test/java/"}
	for _, marker := range markers {
		idx := strings.Index(filePath, marker)
		if idx < 0 {
			continue
		}
		// Take 4 directory levels after the marker
		// com/example/platform/contentservices → distinguishes from com/example/platform/notifications
		rest := filePath[idx+len(marker):]
		parts := strings.SplitN(rest, "/", 5)
		if len(parts) >= 4 {
			return filePath[:idx] + marker + strings.Join(parts[:4], "/") + "/"
		}
	}
	return ""
}

func detectBuildCommand(repoPath string, platform string) (string, []string) {
	switch platform {
	case "java":
		if _, err := os.Stat(filepath.Join(repoPath, "pom.xml")); err == nil {
			return "mvn", []string{"compile", "-q", "-f", filepath.Join(repoPath, "pom.xml")}
		}
		if _, err := os.Stat(filepath.Join(repoPath, "build.gradle")); err == nil {
			return filepath.Join(repoPath, "gradlew"), []string{"compileJava", "-q"}
		}
		if _, err := os.Stat(filepath.Join(repoPath, "build.gradle.kts")); err == nil {
			return filepath.Join(repoPath, "gradlew"), []string{"compileJava", "-q"}
		}
	case "go":
		return "go", []string{"build", "./..."}
	case "typescript":
		if _, err := os.Stat(filepath.Join(repoPath, "tsconfig.json")); err == nil {
			return "npx", []string{"tsc", "--noEmit"}
		}
	case "kotlin":
		if _, err := os.Stat(filepath.Join(repoPath, "build.gradle.kts")); err == nil {
			return filepath.Join(repoPath, "gradlew"), []string{"compileKotlin", "-q"}
		}
	}
	return "", nil
}

func runBuildCheck(repoPath string, command string, args []string) error {
	cmd := exec.Command(command, args...)
	cmd.Dir = repoPath
	cmd.Env = append(os.Environ(), "TERM=dumb")

	var stderr strings.Builder
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		output := stderr.String()
		if len(output) > 2000 {
			output = output[:2000] + "\n... truncated"
		}
		return fmt.Errorf("build check failed: %v\n%s", err, output)
	}
	return nil
}

func attemptCompileFix(llmProvider llm.Provider, g *gitops.GitOps, buildError string, filesChanged []string, investigation *InvestigationContext) bool {
	if len(filesChanged) == 0 {
		return false
	}

	var prompt strings.Builder
	prompt.WriteString("The following build error occurred after code generation. Fix the compilation error.\n\n")
	prompt.WriteString(fmt.Sprintf("Build error:\n%s\n\n", buildError))
	prompt.WriteString("Files that were changed:\n")
	for _, f := range filesChanged {
		content, err := g.ReadFile(f)
		if err != nil {
			continue
		}
		if len(content) > 4000 {
			content = content[:4000] + "\n// ... truncated"
		}
		prompt.WriteString(fmt.Sprintf("\n--- %s ---\n%s\n", f, content))
	}

	prompt.WriteString("\nOutput ONLY valid JSON: {\"fixes\": [{\"file\": \"path\", \"code\": \"complete corrected file\"}]}\n")
	prompt.WriteString("Only fix the compilation error. Do not refactor or change behavior.")

	resp, err := llmProvider.Complete(llm.CompletionRequest{
		SystemPrompt: "You are a compiler error fixer. Output ONLY the JSON with corrected file contents.",
		UserPrompt:   prompt.String(),
		MaxTokens:    8192,
	})
	if err != nil {
		return false
	}

	body := resp.Content
	body = strings.TrimPrefix(body, "```json")
	body = strings.TrimPrefix(body, "```")
	body = strings.TrimSuffix(body, "```")
	body = strings.TrimSpace(body)

	var result struct {
		Fixes []struct {
			File string `json:"file"`
			Code string `json:"code"`
		} `json:"fixes"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return false
	}

	applied := false
	for _, fix := range result.Fixes {
		if fix.File != "" && fix.Code != "" {
			if err := g.WriteFile(fix.File, fix.Code); err == nil {
				applied = true
			}
		}
	}
	return applied
}

func safeFirst(s []string) string {
	if len(s) > 0 {
		return s[0]
	}
	return "(none)"
}

func buildPrepSteps(inv *InvestigationContext, analysis *AnalysisResult) []PlanStep {
	var steps []PlanStep

	if len(inv.ExistingEndpoints) > 0 {
		var endpoints []string
		for _, ep := range inv.ExistingEndpoints {
			endpoints = append(endpoints, fmt.Sprintf("%s %s → %s", ep.Method, ep.Path, ep.Class))
		}
		steps = append(steps, PlanStep{
			Action:  "review",
			File:    "Existing Endpoints",
			Content: fmt.Sprintf("Verify these %d endpoints meet the ticket requirements:\n%s", len(endpoints), strings.Join(endpoints, "\n")),
		})
	}

	if len(inv.BackendAPIs) > 0 {
		var apis []string
		for _, ep := range inv.BackendAPIs {
			apis = append(apis, fmt.Sprintf("%s %s → %s (%s)", ep.Method, ep.Path, ep.Class, ep.File))
		}
		steps = append(steps, PlanStep{
			Action:  "review",
			File:    "Backend APIs for App",
			Content: fmt.Sprintf("Backend provides %d API endpoints for the app to consume:\n%s", len(apis), strings.Join(apis, "\n")),
		})
	}

	if len(inv.ExternalAPIs) > 0 {
		var facades []string
		for _, api := range inv.ExternalAPIs {
			label := "EXTERNAL"
			if api.Internal {
				label = "INTERNAL"
			}
			facades = append(facades, fmt.Sprintf("[%s] %s (%s)", label, api.FacadeClass, api.File))
		}
		steps = append(steps, PlanStep{
			Action:  "review",
			File:    "External Facades",
			Content: fmt.Sprintf("%d facades to verify:\n%s", len(facades), strings.Join(facades, "\n")),
		})
	}

	if analysis != nil {
		if len(analysis.Requirements) > 0 {
			var reqs []string
			for _, r := range analysis.Requirements {
				reqs = append(reqs, fmt.Sprintf("[%s] %s: %s → %s", r.Priority, r.ID, r.Description, r.CodeImpact))
			}
			steps = append(steps, PlanStep{
				Action:  "clarify",
				File:    "Requirements",
				Content: fmt.Sprintf("%d requirements to review with team:\n%s", len(reqs), strings.Join(reqs, "\n")),
			})
		}

		if len(analysis.Unknowns) > 0 {
			steps = append(steps, PlanStep{
				Action:  "clarify",
				File:    "Open Questions",
				Content: fmt.Sprintf("%d unknowns to resolve before implementation:\n- %s", len(analysis.Unknowns), strings.Join(analysis.Unknowns, "\n- ")),
			})
		}

		if len(analysis.EdgeCases) > 0 {
			var ecs []string
			for _, ec := range analysis.EdgeCases {
				ecs = append(ecs, fmt.Sprintf("When: %s → Then: %s", ec.Trigger, ec.Expected))
			}
			steps = append(steps, PlanStep{
				Action:  "verify",
				File:    "Edge Cases",
				Content: fmt.Sprintf("%d edge cases to validate:\n%s", len(ecs), strings.Join(ecs, "\n")),
			})
		}
	}

	if len(inv.Warnings) > 0 {
		steps = append(steps, PlanStep{
			Action:  "resolve",
			File:    "Warnings",
			Content: strings.Join(inv.Warnings, "\n"),
		})
	}

	return steps
}
