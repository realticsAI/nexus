package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anurag/nexus/internal/debug"
	"github.com/anurag/nexus/internal/github"
	"github.com/anurag/nexus/internal/jira"
	"github.com/anurag/nexus/internal/llm"
	"github.com/anurag/nexus/model"
)

func DetectTicketPlatform(issue *jira.Issue) TicketPlatform {
	text := strings.ToLower(issue.Summary + " " + issue.Description + " " + strings.Join(issue.Labels, " ") + " " + strings.Join(issue.Components, " "))

	rnMarkers := []string{" rn ", "| rn |", "| rn", "rn |", "react native", "react-native", "reactnative"}
	for _, m := range rnMarkers {
		if strings.Contains(text, m) {
			return PlatformRN
		}
	}
	iosMarkers := []string{" ios ", "| ios |", "| ios", "ios |", "swift", "swiftui", "xcode", "uikit"}
	for _, m := range iosMarkers {
		if strings.Contains(text, m) {
			return PlatformIOS
		}
	}
	androidMarkers := []string{" android ", "| android |", "| android", "android |", "kotlin", "jetpack", "compose"}
	for _, m := range androidMarkers {
		if strings.Contains(text, m) {
			return PlatformAndroid
		}
	}
	webMarkers := []string{" web ", "| web |", "| web", "web |", "frontend", "react app", "next.js", "nextjs"}
	for _, m := range webMarkers {
		if strings.Contains(text, m) {
			return PlatformWeb
		}
	}

	// Fuzzy: "mobile", "app", "screen", "component", "navigation" without backend signals
	mobileHints := []string{"mobile app", "mobile screen", "mobile component", " app screen", "app component", "navigation screen", "tab screen"}
	backendSignals := []string{"endpoint", "api", "service layer", "repository", "facade", "controller", "kafka", "couchbase", "database", "migration"}
	hasBackendSignal := false
	for _, bs := range backendSignals {
		if strings.Contains(text, bs) {
			hasBackendSignal = true
			break
		}
	}
	if !hasBackendSignal {
		for _, mh := range mobileHints {
			if strings.Contains(text, mh) {
				return PlatformRN // default mobile to RN — most common; will be corrected if iOS/Android repo is indexed but RN isn't
			}
		}
	}

	return PlatformUnknown
}

type weightedKeyword struct {
	word   string
	weight int // title/component=50, label=30, description=1
}

func ResolveServiceKeys(issue *jira.Issue, summaries map[string]*model.ServiceSummary) []string {
	wkw := extractWeightedKeywords(issue)
	debug.Log("agent", "ResolveServiceKeys titleKW=%v", titleKeywords(wkw))

	// Extract high-signal words from title and components for domain affinity
	var domainWords []string
	for _, word := range strings.Fields(issue.Summary) {
		w := strings.ToLower(strings.Trim(word, ".,;:!?\"'()[]{}*|—"))
		if len(w) > 3 && !isStopWord(w) {
			domainWords = append(domainWords, w)
		}
	}
	for _, c := range issue.Components {
		for _, word := range strings.Fields(c) {
			w := strings.ToLower(strings.Trim(word, ".,;:!?\"'()[]{}*|"))
			if len(w) > 3 && !isStopWord(w) {
				domainWords = append(domainWords, w)
			}
		}
	}

	type scored struct {
		key      string
		score    int
		isDomain bool // true if service group matches ticket domain words
	}
	var results []scored
	for key, s := range summaries {
		score := summaryRelevanceWeighted(s, wkw)
		if score == 0 {
			continue
		}

		// Domain affinity: boost services whose group prefix matches ticket domain words.
		// e.g. ticket "Order Notifications..." + service key "order_service:order.service-events"
		// → group "order_service" contains "commerce" → +100 bonus per match.
		// Skip generic infrastructure groups where the match is just a common word
		// (e.g. "notifications:" matching "notifications" from ticket title).
		isDomain := false
		group := key
		if idx := strings.Index(key, ":"); idx > 0 {
			group = key[:idx]
		}
		groupLower := strings.ToLower(strings.ReplaceAll(group, "_", " "))
		if !isInfraGroup(groupLower) {
			for _, dw := range domainWords {
				if strings.Contains(groupLower, dw) {
					score += 100
					isDomain = true
				}
			}
		}

		debug.Log("agent", "ResolveServiceKeys match key=%s score=%d domain=%v", key, score, isDomain)
		results = append(results, scored{key, score, isDomain})
	}

	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].score > results[i].score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
	keys := make([]string, len(results))
	for i, r := range results {
		keys[i] = r.key
	}
	debug.Log("agent", "ResolveServiceKeys matched=%d keys=%v", len(keys), keys)
	return keys
}

func titleKeywords(wkws []weightedKeyword) []string {
	var out []string
	for _, wk := range wkws {
		if wk.weight >= 30 {
			out = append(out, wk.word)
		}
	}
	return out
}

func summaryRelevanceWeighted(s *model.ServiceSummary, wkws []weightedKeyword) int {
	score := 0
	keyLower := strings.ToLower(s.Key)
	for _, wk := range wkws {
		kwLower := strings.ToLower(wk.word)
		if strings.Contains(keyLower, kwLower) && len(kwLower) > 3 {
			score += wk.weight
		}
	}
	for _, skw := range s.Keywords {
		skwLower := strings.ToLower(skw)
		for _, wk := range wkws {
			kwLower := strings.ToLower(wk.word)
			if strings.Contains(skwLower, kwLower) {
				if wk.weight >= 30 {
					score += 5
				} else {
					score++
				}
			}
		}
	}
	return score
}

func extractWeightedKeywords(issue *jira.Issue) []weightedKeyword {
	seen := make(map[string]bool)
	var result []weightedKeyword

	add := func(word string, weight int) {
		w := strings.ToLower(strings.Trim(word, ".,;:!?\"'()[]{}*|"))
		if len(w) <= 3 || isStopWord(w) || seen[w] {
			return
		}
		seen[w] = true
		result = append(result, weightedKeyword{w, weight})
	}

	// Jira project key prefix (e.g. PROJ from PROJ-1007) — domain signal
	if parts := strings.SplitN(issue.Key, "-", 2); len(parts) == 2 && len(parts[0]) >= 2 {
		add(parts[0], 50)
	}

	// Title/summary words — highest signal
	for _, word := range strings.Fields(issue.Summary) {
		add(word, 50)
	}

	// Components — very high signal
	for _, c := range issue.Components {
		for _, word := range strings.Fields(c) {
			add(word, 50)
		}
	}

	// Labels — high signal
	for _, l := range issue.Labels {
		for _, word := range strings.Fields(l) {
			add(word, 30)
		}
	}

	// Acceptance criteria — medium signal (more structured than description)
	for _, word := range strings.Fields(issue.AccCriteria) {
		add(word, 3)
	}

	// Description — low signal (noisy, verbose)
	for _, word := range strings.Fields(issue.Description) {
		add(word, 1)
	}

	return result
}

func FindPlatformRepoFromSummaries(summaries map[string]*model.ServiceSummary, platform TicketPlatform) (string, bool) {
	for key, s := range summaries {
		nameLower := strings.ToLower(key)
		switch platform {
		case PlatformRN:
			if s.Platform.String() == "typescript" {
				if strings.Contains(nameLower, "rn") || strings.Contains(nameLower, "react-native") ||
					strings.Contains(nameLower, "mobile") || strings.Contains(nameLower, "app") {
					return key, true
				}
			}
		case PlatformIOS:
			if s.Platform.String() == "swift" {
				return key, true
			}
		case PlatformAndroid:
			if s.Platform.String() == "kotlin" {
				return key, true
			}
		case PlatformWeb:
			if s.Platform.String() == "typescript" {
				if strings.Contains(nameLower, "web") || strings.Contains(nameLower, "frontend") ||
					strings.Contains(nameLower, "ui") || strings.Contains(nameLower, "portal") {
					return key, true
				}
			}
		}
	}
	for key, s := range summaries {
		nameLower := strings.ToLower(key)
		switch platform {
		case PlatformRN, PlatformWeb:
			if s.Platform.String() == "typescript" && !isBackendTS(nameLower) {
				return key, true
			}
		}
	}
	return "", false
}

func findPlatformRepo(services map[string]*model.ServiceIndex, platform TicketPlatform) (string, bool) {
	// Exact match first
	for key, svc := range services {
		nameLower := strings.ToLower(key)
		switch platform {
		case PlatformRN:
			if svc.Platform.String() == "typescript" {
				if strings.Contains(nameLower, "rn") || strings.Contains(nameLower, "react-native") ||
					strings.Contains(nameLower, "mobile") || strings.Contains(nameLower, "app") {
					return key, true
				}
			}
		case PlatformIOS:
			if svc.Platform.String() == "swift" {
				return key, true
			}
		case PlatformAndroid:
			if svc.Platform.String() == "kotlin" {
				return key, true
			}
		case PlatformWeb:
			if svc.Platform.String() == "typescript" {
				if strings.Contains(nameLower, "web") || strings.Contains(nameLower, "frontend") ||
					strings.Contains(nameLower, "ui") || strings.Contains(nameLower, "portal") {
					return key, true
				}
			}
		}
	}

	// Fuzzy fallback: any TypeScript repo for RN/Web, any mobile repo for RN
	for key, svc := range services {
		nameLower := strings.ToLower(key)
		switch platform {
		case PlatformRN:
			if svc.Platform.String() == "typescript" && !isBackendTS(nameLower) {
				return key, true
			}
		case PlatformWeb:
			if svc.Platform.String() == "typescript" && !isBackendTS(nameLower) {
				return key, true
			}
		}
		_ = nameLower
	}

	return "", false
}

func isBackendTS(name string) bool {
	backendHints := []string{"service", "api", "server", "lambda", "graphql", "gateway"}
	for _, h := range backendHints {
		if strings.Contains(name, h) {
			return true
		}
	}
	return false
}

func Investigate(issue *jira.Issue, services map[string]*model.ServiceIndex, graph *model.Graph, githubOrg string, allServiceKeys []string) *InvestigationContext {
	debug.Log("agent", "Investigate starting services=%d graphEdges=%d allServiceKeys=%d", len(services), len(graph.Edges), len(allServiceKeys))
	ctx := &InvestigationContext{}

	// Platform detection — what kind of ticket is this?
	ctx.TicketPlatform = DetectTicketPlatform(issue)
	debug.Log("agent", "Investigate platform=%s", ctx.TicketPlatform)
	if ctx.TicketPlatform != PlatformUnknown && ctx.TicketPlatform != PlatformBackend {
		repoKey, found := findPlatformRepo(services, ctx.TicketPlatform)
		ctx.PlatformRepoFound = found
		if !found {
			ctx.Warnings = append(ctx.Warnings, fmt.Sprintf("CRITICAL: Ticket is for %s but NO %s repo is indexed. Code analysis is backend-only. Run 'nexus crawl' with the %s repo path to get frontend analysis.", ctx.TicketPlatform, ctx.TicketPlatform, ctx.TicketPlatform))
		} else {
			ctx.Warnings = append(ctx.Warnings, fmt.Sprintf("Platform repo found: %s (for %s ticket)", repoKey, ctx.TicketPlatform))
		}
	}

	wkw := extractWeightedKeywords(issue)

	// Phase 1: Match services by keywords, ranked by relevance
	type scoredService struct {
		key   string
		score int
	}
	var scored []scoredService
	for key, svc := range services {
		s := serviceRelevanceWeighted(svc, wkw)
		if s > 0 {
			scored = append(scored, scoredService{key, s})
		}
	}
	// Sort by score descending
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}
	for _, s := range scored {
		ctx.Services = append(ctx.Services, s.key)
	}

	// Phase 2: Deep class search — find specific classes matching ticket keywords
	for _, svcKey := range ctx.Services {
		svc := services[svcKey]
		for _, unit := range svc.Units {
			relevance := classRelevance(unit, wkw)
			if relevance == 0 {
				continue
			}

			cd := ClassDetail{
				Name:         unit.Name,
				File:         unit.File,
				Service:      svcKey,
				Stereotype:   unit.Stereotype,
				Annotations:  unit.Annotations,
				Dependencies: unit.Dependencies,
				Imports:      unit.Imports,
				Fields:       unit.Fields,
			}
			for _, m := range unit.Methods {
				sig := m.Name
				if m.ReturnType != "" {
					sig = m.ReturnType + " " + sig
				}
				if m.HTTPMethod != "" {
					sig = m.HTTPMethod + " " + m.HTTPPath + " → " + sig
				}
				if len(m.Parameters) > 0 {
					sig += "(" + strings.Join(m.Parameters, ", ") + ")"
				}
				cd.Methods = append(cd.Methods, sig)

				// Extract @RequestHeader names
				for _, param := range m.Parameters {
					if strings.Contains(param, "@RequestHeader") {
						hdr := extractHeaderName(param)
						if hdr != "" {
							cd.Headers = appendUnique(cd.Headers, hdr)
						}
					}
				}
			}
			cd.Endpoints = unit.Endpoints
			ctx.RelatedClasses = append(ctx.RelatedClasses, cd)
			ctx.RelatedFiles = appendUnique(ctx.RelatedFiles, unit.File)
		}
	}

	// Phase 3: Trace call chain — Controller → Service → Repository → Entity
	ctx.CallChain = traceCallChain(ctx.RelatedClasses)

	// Phase 4: Collect existing endpoints with class/file detail (primary module only)
	primaryService := ""
	if len(ctx.Services) > 0 {
		primaryService = ctx.Services[0]
	}
	var moduleDir string
	if primaryService != "" {
		if svc, ok := services[primaryService]; ok {
			moduleDir = primaryModuleDir(svc, primaryService)
		}
	}
	for _, cd := range ctx.RelatedClasses {
		if cd.Stereotype != "rest_controller" || cd.Service != primaryService {
			continue
		}
		if moduleDir != "" && !strings.HasPrefix(cd.File, moduleDir) {
			continue
		}
		for _, ep := range cd.Endpoints {
			ctx.Endpoints = append(ctx.Endpoints, ep)
			parts := strings.SplitN(ep, " ", 2)
			method, path := "", ep
			if len(parts) == 2 {
				method, path = parts[0], parts[1]
			}
			ctx.ExistingEndpoints = append(ctx.ExistingEndpoints, EndpointDetail{
				Method: method,
				Path:   path,
				Class:  cd.Name,
				File:   cd.File,
			})
		}
	}

	// For frontend tickets without platform repo: reclassify endpoints as backend APIs the app consumes
	if ctx.TicketPlatform != PlatformUnknown && ctx.TicketPlatform != PlatformBackend && !ctx.PlatformRepoFound {
		ctx.BackendAPIs = ctx.ExistingEndpoints
		ctx.Warnings = append(ctx.Warnings, fmt.Sprintf("Backend provides %d API endpoints that the %s app can consume — listed in backend_apis", len(ctx.BackendAPIs), ctx.TicketPlatform))
	}

	// Phase 5: Read actual file signatures for top related classes (primary module)
	var primaryClasses []ClassDetail
	for _, c := range ctx.RelatedClasses {
		if c.Service == primaryService && (moduleDir == "" || strings.HasPrefix(c.File, moduleDir)) {
			primaryClasses = append(primaryClasses, c)
		}
	}
	for _, svcKey := range ctx.Services {
		svc := services[svcKey]
		ctx.FileExcerpts = readFileSignatures(svc.Path, primaryClasses, githubOrg)
		ctx.ExistingFiles = listExistingFiles(svc.Path, primaryClasses)
		break // primary service only
	}

	// Phase 6: Detect architectural patterns from code
	ctx.Patterns = detectPatterns(ctx.RelatedClasses, ctx.FileExcerpts)

	// Phase 7: Graph dependencies
	for _, edge := range graph.Edges {
		for _, svc := range ctx.Services {
			if edge.From == svc || edge.To == svc {
				dep := edge.To
				if edge.To == svc {
					dep = edge.From
				}
				ctx.Dependencies = appendUnique(ctx.Dependencies, dep)
			}
		}
	}

	// Phase 8: Detect external API calls from facades/services
	keywords := flatKeywords(wkw)
	if allServiceKeys == nil {
		allServiceKeys = make([]string, 0, len(services))
		for k := range services {
			allServiceKeys = append(allServiceKeys, k)
		}
	}
	ctx.ExternalAPIs = detectExternalAPIs(services, ctx.Services, keywords, allServiceKeys)

	// Phase 9: Scan internal library dependencies (shared JARs in .m2)
	for _, svcKey := range ctx.Services {
		svc := services[svcKey]
		ctx.LibraryAPIs = ScanLibraryAPIs(svc.Path)
		break // primary service only
	}

	return ctx
}

// RefreshPrimaryContext re-builds endpoints, file excerpts, and patterns
// around a new primary service. Called after LLM override changes Services[0].
func RefreshPrimaryContext(ctx *InvestigationContext, services map[string]*model.ServiceIndex, githubOrg string) {
	if len(ctx.Services) == 0 {
		return
	}
	newPrimary := ctx.Services[0]
	svc, ok := services[newPrimary]
	if !ok {
		return
	}

	moduleDir := primaryModuleDir(svc, newPrimary)

	// Rebuild endpoints from new primary
	ctx.Endpoints = nil
	ctx.ExistingEndpoints = nil
	for _, cd := range ctx.RelatedClasses {
		if cd.Stereotype != "rest_controller" || cd.Service != newPrimary {
			continue
		}
		if moduleDir != "" && !strings.HasPrefix(cd.File, moduleDir) {
			continue
		}
		for _, ep := range cd.Endpoints {
			ctx.Endpoints = append(ctx.Endpoints, ep)
			parts := strings.SplitN(ep, " ", 2)
			method, path := "", ep
			if len(parts) == 2 {
				method, path = parts[0], parts[1]
			}
			ctx.ExistingEndpoints = append(ctx.ExistingEndpoints, EndpointDetail{
				Method: method,
				Path:   path,
				Class:  cd.Name,
				File:   cd.File,
			})
		}
	}

	// Rebuild file excerpts from new primary's classes
	var primaryClasses []ClassDetail
	for _, c := range ctx.RelatedClasses {
		if c.Service == newPrimary && (moduleDir == "" || strings.HasPrefix(c.File, moduleDir)) {
			primaryClasses = append(primaryClasses, c)
		}
	}
	ctx.FileExcerpts = readFileSignatures(svc.Path, primaryClasses, githubOrg)
	ctx.ExistingFiles = listExistingFiles(svc.Path, primaryClasses)

	// Rebuild patterns
	ctx.Patterns = detectPatterns(ctx.RelatedClasses, ctx.FileExcerpts)

	// Rebuild library APIs from new primary's dependencies
	ctx.LibraryAPIs = ScanLibraryAPIs(svc.Path)
}

// TagPlanStepsWithService assigns a Service to each PlanStep by matching
// file paths against RelatedClasses from the investigation.
func TagPlanStepsWithService(steps []PlanStep, investigation *InvestigationContext) {
	fileToService := make(map[string]string)
	for _, c := range investigation.RelatedClasses {
		if c.Service != "" && c.File != "" {
			fileToService[c.File] = c.Service
		}
	}

	primary := ""
	if len(investigation.Services) > 0 {
		primary = investigation.Services[0]
	}

	for i := range steps {
		if steps[i].Service != "" {
			continue
		}
		if svc, ok := fileToService[steps[i].File]; ok {
			steps[i].Service = svc
		} else {
			// Match by path prefix — step file might be a new file in a known service's directory
			for file, svc := range fileToService {
				dir := file[:strings.LastIndex(file, "/")+1]
				if dir != "" && strings.HasPrefix(steps[i].File, dir) {
					steps[i].Service = svc
					break
				}
			}
		}
		if steps[i].Service == "" {
			steps[i].Service = primary
		}
	}
}

func detectExternalAPIs(allServices map[string]*model.ServiceIndex, matchedServices []string, keywords []string, allServiceKeys []string) []ExternalAPI {
	var apis []ExternalAPI

	if len(matchedServices) == 0 {
		return apis
	}
	primaryKey := matchedServices[0]
	svc, ok := allServices[primaryKey]
	if !ok {
		return apis
	}

	// Derive primary module directory from service key (handles monorepo sub-projects)
	moduleDir := primaryModuleDir(svc, primaryKey)

	seen := make(map[string]bool)
	for _, unit := range svc.Units {
		// Restrict to primary module directory
		if moduleDir != "" && !strings.HasPrefix(unit.File, moduleDir) {
			continue
		}

		// Skip test classes
		if strings.Contains(unit.File, "/test/") || strings.HasSuffix(unit.Name, "Test") {
			continue
		}

		nameLower := strings.ToLower(unit.Name)
		isFacade := strings.HasSuffix(nameLower, "facade") ||
			strings.HasSuffix(nameLower, "client") ||
			strings.HasSuffix(nameLower, "connector") ||
			strings.HasSuffix(nameLower, "gateway")

		hasApiCalls := len(unit.ApiCalls) > 0
		if !isFacade && !hasApiCalls {
			continue
		}

		if seen[unit.Name] {
			continue
		}
		seen[unit.Name] = true

		api := ExternalAPI{
			FacadeClass: unit.Name,
			File:        unit.File,
			URLs:        unit.ApiCalls,
			ConfigKeys:  unit.ConfigRefs,
		}

		// Check if facade target is another indexed service or external
		for _, url := range unit.ApiCalls {
			urlLower := strings.ToLower(url)
			for _, svcKey := range allServiceKeys {
				svcName := strings.ToLower(svcKey)
				if strings.Contains(urlLower, svcName) || strings.Contains(svcName, extractServiceStem(urlLower)) {
					api.Internal = true
					break
				}
			}
		}

		apis = append(apis, api)
	}

	return apis
}

func primaryModuleDir(svc *model.ServiceIndex, serviceKey string) string {
	// Extract name portion from key (e.g., "ExampleOrg:order_service" → "order_service")
	name := serviceKey
	if idx := strings.LastIndex(serviceKey, ":"); idx >= 0 {
		name = serviceKey[idx+1:]
	}

	// Collect unique top-level directories from units
	dirs := make(map[string]bool)
	for _, u := range svc.Units {
		parts := strings.SplitN(u.File, "/", 2)
		if len(parts) > 0 && parts[0] != "" {
			dirs[parts[0]] = true
		}
	}

	// Single-module repo: only one top-level dir (e.g., "src") — no filtering needed
	if len(dirs) <= 1 {
		return ""
	}

	// Monorepo: try to match service key name against directories
	// "order_service" matches "order.service" (underscore→dot and dot variants)
	nameDot := strings.ReplaceAll(name, "_", ".")
	nameDash := strings.ReplaceAll(name, "_", "-")
	nameLower := strings.ToLower(name)

	for dir := range dirs {
		dirLower := strings.ToLower(dir)
		if dirLower == nameDot || dirLower == nameDash || dirLower == nameLower {
			return dir + "/"
		}
	}
	// Partial match: directory contains the key name
	for dir := range dirs {
		dirLower := strings.ToLower(dir)
		if strings.Contains(dirLower, nameDot) || strings.Contains(nameDot, dirLower) {
			return dir + "/"
		}
	}
	return ""
}

func extractServiceStem(url string) string {
	url = strings.TrimPrefix(url, "/")
	parts := strings.SplitN(url, "/", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

func classRelevance(unit model.Unit, wkws []weightedKeyword) int {
	score := 0
	nameLower := strings.ToLower(unit.Name)

	for _, wk := range wkws {
		kwLower := strings.ToLower(wk.word)
		if len(kwLower) <= 3 {
			continue
		}
		if strings.Contains(nameLower, kwLower) {
			score += wk.weight / 5
		}
	}

	if unit.Stereotype == "rest_controller" || unit.Stereotype == "service" ||
		unit.Stereotype == "repository" || unit.Stereotype == "configuration" {
		for _, wk := range wkws {
			if wk.weight < 10 {
				continue
			}
			kwLower := strings.ToLower(wk.word)
			for _, m := range unit.Methods {
				if strings.Contains(strings.ToLower(m.Name), kwLower) {
					score += 5
				}
			}
			for _, ep := range unit.Endpoints {
				if strings.Contains(strings.ToLower(ep), kwLower) {
					score += 5
				}
			}
		}
	}

	stems := []string{"request", "response", "document", "entity", "dto", "repository", "validator", "mapper"}
	for _, stem := range stems {
		if strings.Contains(nameLower, stem) {
			for _, wk := range wkws {
				if wk.weight >= 30 && strings.Contains(nameLower, strings.ToLower(wk.word)) {
					score += 3
				}
			}
		}
	}

	domainTerms := []string{"cobrand", "badge", "giftcard", "gift-card", "challenge", "offer", "dining", "specialty", "itinerary", "sailing"}
	for _, dt := range domainTerms {
		if strings.Contains(nameLower, dt) {
			found := false
			for _, wk := range wkws {
				if wk.weight >= 10 && strings.Contains(strings.ToLower(wk.word), dt) {
					found = true
					break
				}
			}
			if !found {
				score -= 15
			}
		}
	}

	if score < 0 {
		score = 0
	}

	return score
}

func traceCallChain(classes []ClassDetail) []CallChainLink {
	var chain []CallChainLink

	controllers := filterByStereotype(classes, "rest_controller")
	services := filterByStereotype(classes, "service")
	repos := filterByStereotype(classes, "repository")

	for _, ctrl := range controllers {
		baseName := strings.TrimSuffix(ctrl.Name, "Controller")
		for _, svc := range services {
			if strings.Contains(svc.Name, baseName) {
				chain = append(chain, CallChainLink{
					From: ctrl.Name, To: svc.Name, Via: "injection",
				})
			}
		}
	}
	for _, svc := range services {
		baseName := strings.TrimSuffix(strings.TrimSuffix(svc.Name, "Service"), "ServiceImpl")
		for _, repo := range repos {
			if strings.Contains(repo.Name, baseName) {
				chain = append(chain, CallChainLink{
					From: svc.Name, To: repo.Name, Via: "injection",
				})
			}
		}
	}

	return chain
}

func filterByStereotype(classes []ClassDetail, stereotype string) []ClassDetail {
	var result []ClassDetail
	for _, c := range classes {
		if c.Stereotype == stereotype {
			result = append(result, c)
		}
	}
	return result
}

func readFileSignatures(repoPath string, classes []ClassDetail, githubOrg string) []FileExcerpt {
	var excerpts []FileExcerpt
	seen := make(map[string]bool)
	const maxExcerpts = 25

	// Build name→class lookup for BFS resolution
	byName := make(map[string]ClassDetail)
	for _, c := range classes {
		byName[c.Name] = c
	}

	// Seed: services and controllers (primary logic classes)
	var seeds []ClassDetail
	for _, c := range classes {
		if c.Stereotype == "service" || c.Stereotype == "rest_controller" {
			seeds = append(seeds, c)
		}
	}

	// BFS: walk dependencies 3 levels deep
	queue := make([]ClassDetail, len(seeds))
	copy(queue, seeds)
	visited := make(map[string]bool)
	var ordered []ClassDetail

	for level := 0; level < 3 && len(queue) > 0; level++ {
		var next []ClassDetail
		for _, c := range queue {
			if visited[c.Name] {
				continue
			}
			visited[c.Name] = true
			ordered = append(ordered, c)

			// Follow explicit dependencies (private final injected types)
			for _, dep := range c.Dependencies {
				if !visited[dep] {
					if target, ok := byName[dep]; ok {
						next = append(next, target)
					}
				}
			}

			// Follow method param/return types that match known classes
			for _, methodSig := range c.Methods {
				for name, target := range byName {
					if !visited[name] && strings.Contains(methodSig, name) {
						next = append(next, target)
					}
				}
			}

			// Follow imports that resolve to known classes
			for _, imp := range c.Imports {
				parts := strings.Split(imp, ".")
				shortName := parts[len(parts)-1]
				if !visited[shortName] {
					if target, ok := byName[shortName]; ok {
						next = append(next, target)
					}
				}
			}
		}
		queue = next
	}

	// Also include model/DTO classes that weren't reached by BFS
	// (catches orphan response types referenced only in method bodies)
	for _, c := range classes {
		if visited[c.Name] {
			continue
		}
		nameLower := strings.ToLower(c.Name)
		if strings.Contains(nameLower, "request") || strings.Contains(nameLower, "response") ||
			strings.Contains(nameLower, "document") || strings.Contains(nameLower, "entity") ||
			strings.Contains(nameLower, "dto") || strings.Contains(nameLower, "mapper") ||
			strings.Contains(nameLower, "config") {
			ordered = append(ordered, c)
		}
	}

	// Read file signatures for all discovered classes
	for _, c := range ordered {
		if len(excerpts) >= maxExcerpts {
			break
		}
		if seen[c.File] {
			continue
		}
		seen[c.File] = true

		fullPath := filepath.Join(repoPath, c.File)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			content, err = github.FetchFileContent(repoPath, c.File, githubOrg)
			if err != nil {
				continue
			}
		}

		sig := extractClassSignature(string(content), c.Name)
		if sig != "" {
			excerpts = append(excerpts, FileExcerpt{
				File:      c.File,
				ClassName: c.Name,
				Signature: sig,
			})
		}
	}

	return excerpts
}

func extractClassSignature(content, className string) string {
	lines := strings.Split(content, "\n")
	var sig strings.Builder
	var domainImports []string
	inClass := false
	braceDepth := 0
	methodCount := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Capture package
		if strings.HasPrefix(trimmed, "package ") {
			sig.WriteString(trimmed + "\n")
			continue
		}

		// Collect domain imports (skip java.*, javax.*, standard libs)
		if strings.HasPrefix(trimmed, "import ") {
			imp := strings.TrimSuffix(strings.TrimPrefix(trimmed, "import "), ";")
			imp = strings.TrimPrefix(imp, "static ")
			if isDomainImport(imp) {
				domainImports = append(domainImports, trimmed)
			}
			continue
		}

		// Capture class-level annotations and declaration
		if !inClass {
			if strings.Contains(trimmed, "@") && !strings.HasPrefix(trimmed, "//") {
				sig.WriteString(trimmed + "\n")
			}
			if strings.Contains(line, "class "+className) || strings.Contains(line, "interface "+className) {
				// Write domain imports before class declaration
				if len(domainImports) > 0 {
					for _, imp := range domainImports {
						sig.WriteString(imp + "\n")
					}
					sig.WriteString("\n")
				}
				sig.WriteString(trimmed + "\n")
				inClass = true
				braceDepth = 1
				continue
			}
			continue
		}

		braceDepth += strings.Count(trimmed, "{") - strings.Count(trimmed, "}")

		if braceDepth <= 0 {
			break
		}

		// Capture field declarations (depth 1, no method body)
		if braceDepth == 1 {
			if strings.Contains(trimmed, "private ") || strings.Contains(trimmed, "protected ") ||
				strings.Contains(trimmed, "public ") || strings.Contains(trimmed, "@") {
				if !strings.Contains(trimmed, "{") || strings.HasSuffix(trimmed, "{") {
					sig.WriteString("  " + trimmed + "\n")
					if strings.HasSuffix(trimmed, "{") {
						methodCount++
					}
				}
			}
		}

		if methodCount > 20 {
			sig.WriteString("  // ... more methods\n")
			break
		}
	}

	result := sig.String()
	if len(result) > 4000 {
		result = result[:4000] + "\n  // ... truncated"
	}
	return result
}

var stdlibPrefixes = []string{
	"java.", "javax.", "jakarta.",
	"org.springframework.", "org.junit.", "org.mockito.",
	"org.slf4j.", "org.apache.", "org.reactivestreams.",
	"reactor.", "lombok.", "io.swagger.",
	"com.fasterxml.", "com.google.", "io.netty.",
}

func isDomainImport(imp string) bool {
	for _, prefix := range stdlibPrefixes {
		if strings.HasPrefix(imp, prefix) {
			return false
		}
	}
	return true
}

func listExistingFiles(repoPath string, classes []ClassDetail) []string {
	var existing []string
	seen := make(map[string]bool)
	for _, c := range classes {
		if seen[c.File] {
			continue
		}
		seen[c.File] = true
		fullPath := filepath.Join(repoPath, c.File)
		if _, err := os.Stat(fullPath); err == nil {
			existing = append(existing, c.File)
		}
	}
	return existing
}

func detectPatterns(classes []ClassDetail, excerpts []FileExcerpt) []string {
	var patterns []string

	// Build combined text from method signatures + file excerpts
	allText := collectPatternText(classes, excerpts)

	checks := []struct {
		pattern string
		terms   []string
	}{
		{"reactive:spring-webflux (Mono/Flux)", []string{"Mono<", "Flux<"}},
		{"persistence:couchbase", []string{"CouchbaseRepository", "ReactiveN1qlCouchbaseRepository", "Couchbase"}},
		{"persistence:jpa", []string{"JpaRepository", "CrudRepository"}},
		{"http-client:webclient", []string{"WebClient"}},
		{"messaging:kafka", []string{"KafkaTemplate", "KafkaListener"}},
	}

	for _, check := range checks {
		for _, term := range check.terms {
			if strings.Contains(allText, term) {
				patterns = appendUnique(patterns, check.pattern)
				break
			}
		}
	}

	// Validation from annotations
	for _, c := range classes {
		for _, ann := range c.Annotations {
			if strings.Contains(ann, "Validated") || strings.Contains(ann, "Valid") {
				patterns = appendUnique(patterns, "validation:bean-validation")
				break
			}
		}
	}

	// Collect all unique request headers across controllers
	var allHeaders []string
	for _, c := range classes {
		if c.Stereotype == "rest_controller" {
			for _, h := range c.Headers {
				allHeaders = appendUnique(allHeaders, h)
			}
		}
	}
	if len(allHeaders) > 0 {
		patterns = appendUnique(patterns, "identity:request-headers ("+strings.Join(allHeaders, ", ")+")")
	}

	return patterns
}

func collectPatternText(classes []ClassDetail, excerpts []FileExcerpt) string {
	var sb strings.Builder
	for _, c := range classes {
		for _, m := range c.Methods {
			sb.WriteString(m)
			sb.WriteByte('\n')
		}
		for _, ann := range c.Annotations {
			sb.WriteString(ann)
			sb.WriteByte('\n')
		}
	}
	for _, e := range excerpts {
		sb.WriteString(e.Signature)
		sb.WriteByte('\n')
	}
	return sb.String()
}

func extractHeaderName(param string) string {
	idx := strings.Index(param, "@RequestHeader")
	if idx < 0 {
		return ""
	}
	rest := param[idx+len("@RequestHeader"):]
	rest = strings.TrimSpace(rest)
	if len(rest) == 0 || rest[0] != '(' {
		parts := strings.Fields(param)
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
		return ""
	}
	end := strings.IndexByte(rest, ')')
	if end < 0 {
		return ""
	}
	inner := rest[1:end]

	// Extract just the first quoted string — the header name
	firstQuote := strings.IndexByte(inner, '"')
	if firstQuote < 0 {
		// No quotes — might be a constant like ACCOUNT_ID
		val := strings.TrimSpace(inner)
		if strings.Contains(val, "=") {
			eqIdx := strings.IndexByte(val, '=')
			val = strings.TrimSpace(val[eqIdx+1:])
		}
		if commaIdx := strings.IndexByte(val, ','); commaIdx >= 0 {
			val = strings.TrimSpace(val[:commaIdx])
		}
		return val
	}
	secondQuote := strings.IndexByte(inner[firstQuote+1:], '"')
	if secondQuote < 0 {
		return ""
	}
	return inner[firstQuote+1 : firstQuote+1+secondQuote]
}

func extractKeywords(issue *jira.Issue) []string {
	var keywords []string
	keywords = append(keywords, issue.Components...)
	keywords = append(keywords, issue.Labels...)

	for _, field := range []string{issue.Summary, issue.Description} {
		for _, word := range strings.Fields(field) {
			w := strings.ToLower(strings.Trim(word, ".,;:!?\"'()[]{}*|"))
			if len(w) > 3 && !isStopWord(w) {
				keywords = append(keywords, w)
			}
		}
	}

	// Deduplicate
	seen := make(map[string]bool)
	var unique []string
	for _, kw := range keywords {
		lower := strings.ToLower(kw)
		if !seen[lower] {
			seen[lower] = true
			unique = append(unique, kw)
		}
	}
	return unique
}

func matchesService(svc *model.ServiceIndex, keywords []string) bool {
	return serviceRelevance(svc, keywords) > 0
}

func serviceRelevance(svc *model.ServiceIndex, keywords []string) int {
	score := 0
	keyLower := strings.ToLower(svc.Key)
	for _, kw := range keywords {
		if strings.Contains(keyLower, strings.ToLower(kw)) {
			score += 10
		}
	}
	for _, unit := range svc.Units {
		nameLower := strings.ToLower(unit.Name)
		for _, kw := range keywords {
			if strings.Contains(nameLower, strings.ToLower(kw)) {
				score++
			}
		}
	}
	return score
}

func serviceRelevanceWeighted(svc *model.ServiceIndex, wkws []weightedKeyword) int {
	score := 0
	keyLower := strings.ToLower(svc.Key)
	for _, wk := range wkws {
		kwLower := strings.ToLower(wk.word)
		if strings.Contains(keyLower, kwLower) && len(kwLower) > 3 {
			score += wk.weight
		}
	}
	for _, unit := range svc.Units {
		nameLower := strings.ToLower(unit.Name)
		for _, wk := range wkws {
			kwLower := strings.ToLower(wk.word)
			if strings.Contains(nameLower, kwLower) {
				if wk.weight >= 30 {
					score += 3
				} else {
					score++
				}
			}
		}
	}
	return score
}

func flatKeywords(wkws []weightedKeyword) []string {
	out := make([]string, len(wkws))
	for i, wk := range wkws {
		out[i] = wk.word
	}
	return out
}

func matchesAny(name string, keywords []string) bool {
	nameLower := strings.ToLower(name)
	for _, kw := range keywords {
		if strings.Contains(nameLower, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

func isStopWord(w string) bool {
	stops := map[string]bool{
		"the": true, "and": true, "for": true, "from": true, "with": true,
		"that": true, "this": true, "when": true, "should": true, "would": true,
		"could": true, "into": true, "have": true, "been": true, "will": true,
		"does": true, "what": true, "which": true, "where": true, "after": true,
		"before": true, "about": true, "then": true, "than": true, "also": true,
		"their": true, "they": true, "there": true, "these": true, "those": true,
		"more": true, "other": true, "some": true, "each": true, "within": true,
		"between": true, "being": true, "existing": true, "given": true,
		// Jira category labels — generic words that appear in pipe-separated ticket titles
		// (e.g. "Commerce | Services | Push Notifications") but don't help repo matching
		"services": true, "service": true, "frontend": true, "backend": true,
		"mobile": true, "platform": true, "system": true, "update": true,
		"feature": true, "issue": true, "request": true, "ticket": true,
		"changes": true, "implementation": true, "development": true, "support": true,
		"application": true, "component": true, "module": true, "project": true,
	}
	return stops[w]
}

// isInfraGroup returns true for generic infrastructure group prefixes that
// shouldn't get domain affinity boosts. These groups contain shared platform
// services (notifications, graphql, megatron) not domain-specific business logic.
func isInfraGroup(groupLower string) bool {
	infraGroups := map[string]bool{
		"notifications": true, "graphql": true, "megatron": true,
		"guest": true, "core": true, "exampleorg": true,
	}
	return infraGroups[groupLower]
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

func (ctx *InvestigationContext) Summary() string {
	libClassCount := 0
	for _, lib := range ctx.LibraryAPIs {
		libClassCount += len(lib.Classes)
	}
	return fmt.Sprintf("%d services, %d classes, %d endpoints (%d existing), %d deps, %d excerpts, %d figma, %d ext-apis, %d linked-issues, %d lib-classes, patterns: %v",
		len(ctx.Services), len(ctx.RelatedClasses), len(ctx.Endpoints), len(ctx.ExistingEndpoints),
		len(ctx.Dependencies), len(ctx.FileExcerpts), len(ctx.FigmaDesigns), len(ctx.ExternalAPIs), len(ctx.LinkedIssues), libClassCount, ctx.Patterns)
}

// ValidateServiceSelection uses the LLM to verify which candidate service
// actually OWNS the behavior described in the ticket. Keyword scoring can
// pick the wrong repo when generic words (e.g. "services") inflate scores
// for repos that merely reference/display the data rather than own the logic.
func ValidateServiceSelection(llmProvider llm.Provider, issue *jira.Issue, candidates []string, summaries map[string]*model.ServiceSummary) ([]string, int, error) {
	if len(candidates) < 2 {
		return candidates, 0, nil
	}

	cap := 5
	if len(candidates) < cap {
		cap = len(candidates)
	}
	top := candidates[:cap]

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Ticket: %s\nSummary: %s\n", issue.Key, issue.Summary))
	desc := issue.Description
	if len(desc) > 1500 {
		desc = desc[:1500]
	}
	sb.WriteString(fmt.Sprintf("Description: %s\n\n", desc))

	sb.WriteString("Candidate services (ranked by keyword score):\n")
	for i, key := range top {
		s := summaries[key]
		kw := ""
		if s != nil && len(s.Keywords) > 0 {
			kwSlice := s.Keywords
			if len(kwSlice) > 15 {
				kwSlice = kwSlice[:15]
			}
			kw = strings.Join(kwSlice, ", ")
		}
		path := ""
		units := 0
		if s != nil {
			path = s.Path
			units = s.UnitCount
		}
		sb.WriteString(fmt.Sprintf("  %d. key=%s  path=%s  units=%d  keywords=[%s]\n", i+1, key, path, units, kw))
	}

	systemPrompt := `You are a service-ownership expert. Given a Jira ticket and a list of candidate services from a monorepo, determine which service OWNS the behavior described in the ticket.

OWNERSHIP means: which service is the SOURCE of the behavior that needs to change? Not which service displays, consumes, or maps the data — but which service creates, schedules, sends, or enforces the logic the ticket is about.

Examples:
- "Notifications not expiring" → the service that SENDS/SCHEDULES notifications owns this, not the one that displays them
- "Cache not invalidating" → the service that manages the cache owns this
- "Wrong response format" → the service that constructs the response owns this

Output ONLY valid JSON:
{"primary_service": "the exact service key", "reasoning": "one sentence explaining why this service owns the behavior"}`

	resp, err := llmProvider.Complete(llm.CompletionRequest{
		SystemPrompt: systemPrompt,
		UserPrompt:   sb.String(),
		MaxTokens:    256,
	})
	if err != nil {
		debug.Log("agent", "ValidateServiceSelection LLM error: %v", err)
		return candidates, 0, err
	}

	var result struct {
		PrimaryService string `json:"primary_service"`
		Reasoning      string `json:"reasoning"`
	}
	content := strings.TrimSpace(resp.Content)
	if idx := strings.Index(content, "{"); idx >= 0 {
		content = content[idx:]
	}
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		debug.Log("agent", "ValidateServiceSelection parse error: %v content=%s", err, content)
		return candidates, resp.TokensUsed, fmt.Errorf("parse error: %w", err)
	}

	if result.PrimaryService == "" {
		return candidates, resp.TokensUsed, fmt.Errorf("LLM returned empty primary_service")
	}

	// Check if LLM picked a different service than keyword scoring
	if result.PrimaryService == candidates[0] {
		debug.Log("agent", "ValidateServiceSelection confirmed: %s — %s", result.PrimaryService, result.Reasoning)
		return candidates, resp.TokensUsed, nil
	}

	// LLM disagrees — reorder the list with the LLM's pick first
	found := false
	reordered := []string{result.PrimaryService}
	for _, c := range candidates {
		if c == result.PrimaryService {
			found = true
			continue
		}
		reordered = append(reordered, c)
	}

	if !found {
		debug.Log("agent", "ValidateServiceSelection: LLM picked %s which is not in candidates, ignoring", result.PrimaryService)
		return candidates, resp.TokensUsed, nil
	}

	debug.Log("agent", "ValidateServiceSelection REORDERED: %s → %s — %s", candidates[0], result.PrimaryService, result.Reasoning)
	return reordered, resp.TokensUsed, nil
}
