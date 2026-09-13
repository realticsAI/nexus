package figma

import (
	"strings"
)

type ScreenAnalysis struct {
	FrameName    string            `json:"frame_name"`
	FrameID      string            `json:"frame_id"`
	ScreenType   string            `json:"screen_type"`
	Elements     []UIElement       `json:"elements"`
	DataSources  []DataSource      `json:"data_sources"`
	EntryPoints  []EntryPoint      `json:"entry_points"`
	Interactions []Interaction     `json:"interactions"`
	EdgeCases    []ScreenEdgeCase  `json:"edge_cases"`
	Summary      string            `json:"summary"`
}

type UIElement struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Text       string `json:"text,omitempty"`
	Category   string `json:"category"`
	DataNeeded string `json:"data_needed,omitempty"`
	LogicOwner string `json:"logic_owner,omitempty"`
}

type DataSource struct {
	Field       string `json:"field"`
	Source      string `json:"source"`
	Description string `json:"description"`
}

type EntryPoint struct {
	Label  string `json:"label"`
	Action string `json:"action"`
	Target string `json:"target,omitempty"`
}

type Interaction struct {
	Trigger  string `json:"trigger"`
	Action   string `json:"action"`
	SavesTo  string `json:"saves_to,omitempty"`
}

type ScreenEdgeCase struct {
	Condition string `json:"condition"`
	UIState   string `json:"ui_state"`
	Category  string `json:"category"`
}

type ScreenClassification struct {
	Screens       []ScreenAnalysis `json:"screens"`
	DataFlowMap   []DataFlow       `json:"data_flow_map"`
	BackendNeeds  []BackendNeed    `json:"backend_needs"`
	FrontendLogic []FrontendLogic  `json:"frontend_logic"`
	OpenQuestions []string         `json:"open_questions"`
}

type DataFlow struct {
	UIElement string `json:"ui_element"`
	LabelFrom string `json:"label_from"`
	ValueFrom string `json:"value_from"`
	ListFrom  string `json:"list_from,omitempty"`
}

type BackendNeed struct {
	Endpoint    string `json:"endpoint"`
	Method      string `json:"method"`
	Description string `json:"description"`
	Fields      []string `json:"fields,omitempty"`
}

type FrontendLogic struct {
	Logic       string `json:"logic"`
	Description string `json:"description"`
}

var screenTypeKeywords = map[string][]string{
	"error":        {"error", "oops", "something went wrong", "try again", "retry", "failed"},
	"empty":        {"no results", "nothing here", "get started", "no items", "empty"},
	"loading":      {"loading", "please wait", "spinner"},
	"success":      {"success", "done", "completed", "confirmed", "saved"},
	"onboarding":   {"welcome", "get started", "set up", "introduction"},
	"list":         {"select", "choose", "pick", "apply", "search"},
	"detail":       {"details", "view", "summary"},
	"form":         {"save", "submit", "update", "preferences", "settings"},
	"confirmation": {"confirm", "are you sure", "cancel", "proceed"},
}

var elementCategoryPatterns = map[string][]string{
	"navigation":        {"back", "close", "chevron", "arrow", "nav", "header", "toolbar"},
	"segmented_control": {"segment", "tab", "toggle_group", "switch_group", "pill"},
	"toggle":            {"toggle", "switch", "on_off"},
	"button":            {"button", "cta", "btn", "action"},
	"input":             {"input", "field", "text_field", "search", "edit"},
	"list_item":         {"row", "cell", "list_item", "card", "tile"},
	"label":             {"label", "title", "header", "heading", "section", "caption", "subtitle"},
	"image":             {"image", "icon", "avatar", "logo", "illustration", "photo"},
	"badge":             {"badge", "chip", "tag", "count"},
	"modal":             {"modal", "dialog", "popup", "overlay", "bottom_sheet", "sheet"},
}

func AnalyzeScreen(node *Node) *ScreenAnalysis {
	if node == nil {
		return nil
	}

	analysis := &ScreenAnalysis{
		FrameName: node.Name,
		FrameID:   node.ID,
	}

	allText := collectAllText(node)
	analysis.ScreenType = classifyScreen(node.Name, allText)

	walkElements(node, analysis, 0, 15)

	analysis.EntryPoints = detectEntryPoints(analysis.Elements)
	analysis.Interactions = detectInteractions(analysis.Elements)
	analysis.EdgeCases = detectEdgeCases(analysis.ScreenType, analysis.Elements, allText)
	analysis.DataSources = inferDataSources(analysis.Elements)
	analysis.Summary = buildSummary(analysis)

	return analysis
}

func AnalyzeAllScreens(root *Node) *ScreenClassification {
	if root == nil {
		return nil
	}

	result := &ScreenClassification{}
	frames := collectFrames(root)

	for _, frame := range frames {
		screen := AnalyzeScreen(frame)
		if screen != nil {
			result.Screens = append(result.Screens, *screen)
		}
	}

	result.DataFlowMap = buildDataFlowMap(result.Screens)
	result.BackendNeeds = inferBackendNeeds(result.Screens)
	result.FrontendLogic = inferFrontendLogic(result.Screens)
	result.OpenQuestions = detectOpenQuestions(result.Screens)

	return result
}

func collectFrames(node *Node) []*Node {
	var frames []*Node
	if node.Type == "FRAME" || node.Type == "COMPONENT" || node.Type == "COMPONENT_SET" {
		frames = append(frames, node)
	}
	for _, child := range node.Children {
		if child.Type == "CANVAS" || child.Type == "DOCUMENT" {
			frames = append(frames, collectFrames(child)...)
		} else if child.Type == "FRAME" || child.Type == "COMPONENT" || child.Type == "COMPONENT_SET" {
			frames = append(frames, child)
		}
	}
	return frames
}

func walkElements(node *Node, analysis *ScreenAnalysis, depth, maxDepth int) {
	if node == nil || depth > maxDepth {
		return
	}

	elem := classifyElement(node)
	if elem != nil {
		analysis.Elements = append(analysis.Elements, *elem)
	}

	for _, child := range node.Children {
		walkElements(child, analysis, depth+1, maxDepth)
	}
}

func classifyElement(node *Node) *UIElement {
	if node == nil {
		return nil
	}

	nameLower := strings.ToLower(node.Name)

	// Skip layout containers that aren't meaningful UI elements
	if node.Type == "FRAME" && len(node.Children) > 0 && node.Characters == "" {
		cat := matchCategory(nameLower)
		if cat == "" {
			return nil
		}
	}

	elem := &UIElement{
		Name: node.Name,
		Type: node.Type,
	}

	if node.Characters != "" {
		elem.Text = node.Characters
	}

	elem.Category = matchCategory(nameLower)
	if elem.Category == "" {
		switch node.Type {
		case "TEXT":
			elem.Category = "text"
		case "INSTANCE":
			elem.Category = "component"
		case "VECTOR", "BOOLEAN_OPERATION":
			elem.Category = "icon"
		default:
			if len(node.Children) == 0 && node.Characters == "" {
				return nil
			}
			elem.Category = "container"
		}
	}

	elem.LogicOwner = inferLogicOwner(elem)
	elem.DataNeeded = inferDataNeeded(elem)

	return elem
}

func matchCategory(nameLower string) string {
	for category, keywords := range elementCategoryPatterns {
		for _, kw := range keywords {
			if strings.Contains(nameLower, kw) {
				return category
			}
		}
	}
	return ""
}

func classifyScreen(frameName string, allText []string) string {
	combined := strings.ToLower(frameName)
	for _, t := range allText {
		combined += " " + strings.ToLower(t)
	}

	bestType := "main"
	bestScore := 0
	for screenType, keywords := range screenTypeKeywords {
		score := 0
		for _, kw := range keywords {
			if strings.Contains(combined, kw) {
				score++
			}
		}
		if score > bestScore {
			bestScore = score
			bestType = screenType
		}
	}
	return bestType
}

func collectAllText(node *Node) []string {
	return ExtractAllText(node, 20)
}

func detectEntryPoints(elements []UIElement) []EntryPoint {
	var entries []EntryPoint
	for _, elem := range elements {
		if elem.Category == "navigation" {
			entries = append(entries, EntryPoint{
				Label:  elem.Name,
				Action: "navigate",
			})
		}
		if elem.Category == "button" || elem.Category == "segmented_control" {
			entries = append(entries, EntryPoint{
				Label:  displayText(elem),
				Action: "tap",
			})
		}
	}
	return entries
}

func detectInteractions(elements []UIElement) []Interaction {
	var interactions []Interaction
	for _, elem := range elements {
		switch elem.Category {
		case "segmented_control":
			interactions = append(interactions, Interaction{
				Trigger: "tap segment: " + displayText(elem),
				Action:  "select_option",
				SavesTo: "backend_preference",
			})
		case "toggle":
			interactions = append(interactions, Interaction{
				Trigger: "toggle: " + displayText(elem),
				Action:  "toggle_boolean",
				SavesTo: "backend_preference",
			})
		case "button":
			text := displayText(elem)
			textLower := strings.ToLower(text)
			action := "navigate"
			savesTo := ""
			if strings.Contains(textLower, "save") || strings.Contains(textLower, "update") ||
				strings.Contains(textLower, "apply") || strings.Contains(textLower, "submit") {
				action = "save"
				savesTo = "backend_api"
			} else if strings.Contains(textLower, "reset") || strings.Contains(textLower, "clear") {
				action = "reset"
			} else if strings.Contains(textLower, "retry") || strings.Contains(textLower, "refresh") {
				action = "retry"
			}
			interactions = append(interactions, Interaction{
				Trigger: "tap: " + text,
				Action:  action,
				SavesTo: savesTo,
			})
		case "list_item":
			interactions = append(interactions, Interaction{
				Trigger: "tap row: " + displayText(elem),
				Action:  "navigate_to_list",
			})
		}
	}
	return interactions
}

func detectEdgeCases(screenType string, elements []UIElement, allText []string) []ScreenEdgeCase {
	var edgeCases []ScreenEdgeCase
	combinedText := strings.ToLower(strings.Join(allText, " "))

	// Detect from actual screen content — each element type implies specific edge cases
	for _, elem := range elements {
		switch elem.Category {
		case "toggle":
			edgeCases = append(edgeCases, ScreenEdgeCase{
				Condition: "'" + displayText(elem) + "' has no prior saved state (first visit)",
				UIState:   "Show default off state or neutral",
				Category:  "default_state",
			})
		case "segmented_control":
			edgeCases = append(edgeCases, ScreenEdgeCase{
				Condition: "No segment selected for '" + elem.Name + "' (first visit)",
				UIState:   "No segment highlighted or default to neutral",
				Category:  "default_state",
			})
		case "list_item":
			edgeCases = append(edgeCases, ScreenEdgeCase{
				Condition: "Saved selections for '" + elem.Name + "' no longer exist in reference data",
				UIState:   "Skip stale codes, show only valid items",
				Category:  "data_staleness",
			})
		case "button":
			textLower := strings.ToLower(displayText(elem))
			if strings.Contains(textLower, "save") || strings.Contains(textLower, "update") || strings.Contains(textLower, "apply") {
				edgeCases = append(edgeCases, ScreenEdgeCase{
					Condition: "'" + displayText(elem) + "' API call fails or times out",
					UIState:   "Show save error, keep user's input intact",
					Category:  "resilience",
				})
			}
			if strings.Contains(textLower, "retry") || strings.Contains(textLower, "try again") || strings.Contains(textLower, "refresh") {
				edgeCases = append(edgeCases, ScreenEdgeCase{
					Condition: "Retry via '" + displayText(elem) + "' after error",
					UIState:   "Re-fetch data, show loading then result or error again",
					Category:  "resilience",
				})
			}
		}
	}

	// Detect from screen type — only if we actually see evidence
	if screenType == "error" {
		edgeCases = append(edgeCases, ScreenEdgeCase{
			Condition: "Multiple consecutive retries fail",
			UIState:   "Keep showing error screen, don't crash or loop",
			Category:  "resilience",
		})
	}
	if screenType == "list" {
		edgeCases = append(edgeCases, ScreenEdgeCase{
			Condition: "Reference API returns empty list",
			UIState:   "Show empty state with guidance",
			Category:  "empty_state",
		})
	}

	// Detect from text patterns in actual UI content
	if strings.Contains(combinedText, "select up to") || strings.Contains(combinedText, "maximum") || strings.Contains(combinedText, "max ") {
		edgeCases = append(edgeCases, ScreenEdgeCase{
			Condition: "User at selection limit tries to add more",
			UIState:   "Remaining items disabled or show limit toast",
			Category:  "validation",
		})
	}
	if strings.Contains(combinedText, "rank") || strings.Contains(combinedText, "reorder") || strings.Contains(combinedText, "drag") || strings.Contains(combinedText, "favorite") {
		edgeCases = append(edgeCases, ScreenEdgeCase{
			Condition: "User reorders items — order must persist across save",
			UIState:   "Array order saved as rank (position 0 = top choice)",
			Category:  "ordering",
		})
	}
	if strings.Contains(combinedText, "loading") || strings.Contains(combinedText, "please wait") {
		edgeCases = append(edgeCases, ScreenEdgeCase{
			Condition: "API response is slow",
			UIState:   "Show loading skeleton or spinner as seen in screen",
			Category:  "loading",
		})
	}

	// Detect navigation edge cases from actual nav elements
	hasNavBack := false
	hasSaveCTA := false
	for _, elem := range elements {
		if elem.Category == "navigation" {
			hasNavBack = true
		}
		if elem.Category == "button" {
			textLower := strings.ToLower(displayText(elem))
			if strings.Contains(textLower, "save") || strings.Contains(textLower, "update") {
				hasSaveCTA = true
			}
		}
	}
	if hasNavBack && hasSaveCTA {
		edgeCases = append(edgeCases, ScreenEdgeCase{
			Condition: "User taps back with unsaved changes",
			UIState:   "Discard changes or prompt to save (check UX spec)",
			Category:  "navigation",
		})
	}

	return edgeCases
}

func inferDataSources(elements []UIElement) []DataSource {
	var sources []DataSource
	for _, elem := range elements {
		if elem.Category == "label" || elem.Category == "text" {
			if elem.Text != "" {
				sources = append(sources, DataSource{
					Field:       elem.Name,
					Source:      "cms",
					Description: "Label text: \"" + truncate(elem.Text, 60) + "\"",
				})
			}
		}
		if elem.Category == "segmented_control" {
			sources = append(sources, DataSource{
				Field:       elem.Name,
				Source:      "cms+backend",
				Description: "Option labels from CMS, selected state from backend",
			})
		}
		if elem.Category == "toggle" {
			sources = append(sources, DataSource{
				Field:       elem.Name,
				Source:      "backend",
				Description: "Boolean state from backend preference store",
			})
		}
		if elem.Category == "list_item" {
			sources = append(sources, DataSource{
				Field:       elem.Name,
				Source:      "reference_api+backend",
				Description: "List items from reference API, selections from backend",
			})
		}
	}
	return sources
}

func buildDataFlowMap(screens []ScreenAnalysis) []DataFlow {
	var flows []DataFlow
	for _, screen := range screens {
		for _, elem := range screen.Elements {
			if elem.Category == "label" || elem.Category == "text" {
				flows = append(flows, DataFlow{
					UIElement: elem.Name,
					LabelFrom: "CMS/AEM",
					ValueFrom: "static",
				})
			}
			if elem.Category == "segmented_control" {
				flows = append(flows, DataFlow{
					UIElement: elem.Name,
					LabelFrom: "CMS/AEM",
					ValueFrom: "backend_preference",
				})
			}
			if elem.Category == "toggle" {
				flows = append(flows, DataFlow{
					UIElement: elem.Name,
					LabelFrom: "CMS/AEM",
					ValueFrom: "backend_preference",
				})
			}
			if elem.Category == "list_item" {
				flows = append(flows, DataFlow{
					UIElement: elem.Name,
					LabelFrom: "reference_api",
					ValueFrom: "backend_preference",
					ListFrom:  "reference_api",
				})
			}
		}
	}
	return flows
}

func inferBackendNeeds(screens []ScreenAnalysis) []BackendNeed {
	var needs []BackendNeed
	needsGet := false
	needsPatch := false
	var patchFields []string

	for _, screen := range screens {
		for _, interaction := range screen.Interactions {
			if interaction.Action == "save" {
				needsPatch = true
			}
		}
		for _, elem := range screen.Elements {
			if elem.Category == "segmented_control" || elem.Category == "toggle" {
				needsGet = true
				needsPatch = true
				patchFields = append(patchFields, elem.Name)
			}
			if elem.Category == "list_item" && screen.ScreenType == "list" {
				needsGet = true
				needsPatch = true
				patchFields = append(patchFields, elem.Name)
			}
		}
	}

	if needsGet {
		needs = append(needs, BackendNeed{
			Endpoint:    "/preferences",
			Method:      "GET",
			Description: "Fetch saved preferences to populate UI state",
		})
	}
	if needsPatch {
		needs = append(needs, BackendNeed{
			Endpoint:    "/preferences",
			Method:      "PATCH",
			Description: "Save updated preferences (partial update)",
			Fields:      dedup(patchFields),
		})
	}

	hasError := false
	for _, screen := range screens {
		if screen.ScreenType == "error" {
			hasError = true
			break
		}
	}
	if hasError {
		needs = append(needs, BackendNeed{
			Endpoint:    "/preferences",
			Method:      "error_handling",
			Description: "Error response contract needed for resilience screens",
		})
	}

	return needs
}

func inferFrontendLogic(screens []ScreenAnalysis) []FrontendLogic {
	var logic []FrontendLogic

	screenTypes := make(map[string]bool)
	hasSegmented := false
	hasToggle := false
	hasList := false

	for _, screen := range screens {
		screenTypes[screen.ScreenType] = true
		for _, elem := range screen.Elements {
			switch elem.Category {
			case "segmented_control":
				hasSegmented = true
			case "toggle":
				hasToggle = true
			case "list_item":
				hasList = true
			}
		}
	}

	if hasSegmented {
		logic = append(logic, FrontendLogic{
			Logic:       "segmented_control_state",
			Description: "Track selected segment, map to enum, sync with saved preference",
		})
	}
	if hasToggle {
		logic = append(logic, FrontendLogic{
			Logic:       "toggle_state",
			Description: "Track boolean toggle state, sync with saved preference",
		})
	}
	if hasList {
		logic = append(logic, FrontendLogic{
			Logic:       "list_selection_and_ranking",
			Description: "Track selected items + order (drag/reorder), enforce max selection limit",
		})
		logic = append(logic, FrontendLogic{
			Logic:       "code_to_label_resolution",
			Description: "Resolve backend codes to display names using reference API data",
		})
	}
	if screenTypes["error"] {
		logic = append(logic, FrontendLogic{
			Logic:       "error_state_handling",
			Description: "Show error screen on API failure, retry on tap",
		})
	}
	if screenTypes["loading"] {
		logic = append(logic, FrontendLogic{
			Logic:       "loading_skeleton",
			Description: "Show loading state while APIs respond",
		})
	}
	if len(screens) > 1 {
		logic = append(logic, FrontendLogic{
			Logic:       "multi_screen_navigation",
			Description: "Navigate between main form and sub-list screens, preserve state",
		})
		logic = append(logic, FrontendLogic{
			Logic:       "dirty_state_tracking",
			Description: "Track unsaved changes across screens to enable/disable save CTA",
		})
	}

	logic = append(logic, FrontendLogic{
		Logic:       "feature_flag_gating",
		Description: "Hide entry point when feature flag is OFF (safe default)",
	})

	return logic
}

func detectOpenQuestions(screens []ScreenAnalysis) []string {
	var questions []string
	seen := make(map[string]bool)

	add := func(q string) {
		if !seen[q] {
			seen[q] = true
			questions = append(questions, q)
		}
	}

	for _, screen := range screens {
		// Questions based on what we actually found in each screen
		for _, elem := range screen.Elements {
			switch elem.Category {
			case "segmented_control":
				add("What is the default selected segment for '" + elem.Name + "' when user has no saved preference?")
			case "toggle":
				add("What is the default toggle state for '" + displayText(elem) + "' on first visit?")
			case "list_item":
				add("What is the max selection limit for '" + elem.Name + "' and is it brand-specific?")
			}
		}

		for _, interaction := range screen.Interactions {
			if interaction.Action == "save" {
				add("What happens if save fails for '" + interaction.Trigger + "' — show inline error or full error screen?")
			}
		}

		for _, ec := range screen.EdgeCases {
			switch ec.Category {
			case "data_staleness":
				add("How should the UI handle saved codes that no longer exist in reference data?")
			case "ordering":
				add("Is ranking by selection order (first picked = rank 1) or by drag-to-reorder?")
			}
		}

		switch screen.ScreenType {
		case "error":
			add("Does the error screen in '" + screen.FrameName + "' come from CMS content or is it a generic system error?")
		case "list":
			add("What empty state should '" + screen.FrameName + "' show when no items match or are available?")
		}
	}

	// Cross-screen questions
	if len(screens) > 1 {
		hasForm := false
		hasList := false
		for _, s := range screens {
			if s.ScreenType == "form" {
				hasForm = true
			}
			if s.ScreenType == "list" {
				hasList = true
			}
		}
		if hasForm && hasList {
			add("When navigating from list back to form, should unsaved list selections be preserved or discarded?")
		}
	}

	return questions
}

func inferLogicOwner(elem *UIElement) string {
	switch elem.Category {
	case "label", "text":
		return "cms"
	case "segmented_control", "toggle":
		return "frontend+backend"
	case "button":
		textLower := strings.ToLower(displayText(*elem))
		if strings.Contains(textLower, "save") || strings.Contains(textLower, "update") || strings.Contains(textLower, "apply") {
			return "frontend→backend"
		}
		return "frontend"
	case "list_item":
		return "reference_api+backend"
	case "navigation":
		return "frontend"
	case "image", "icon":
		return "cms"
	default:
		return "frontend"
	}
}

func inferDataNeeded(elem *UIElement) string {
	switch elem.Category {
	case "label", "text":
		return "cms_content"
	case "segmented_control":
		return "option_labels(cms) + selected_value(backend)"
	case "toggle":
		return "label(cms) + boolean_state(backend)"
	case "list_item":
		return "item_list(reference_api) + selections(backend)"
	case "button":
		return "cta_text(cms)"
	default:
		return ""
	}
}

func displayText(elem UIElement) string {
	if elem.Text != "" {
		return elem.Text
	}
	return elem.Name
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func dedup(in []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func buildSummary(analysis *ScreenAnalysis) string {
	var parts []string
	parts = append(parts, analysis.ScreenType+" screen")

	elemCounts := make(map[string]int)
	for _, elem := range analysis.Elements {
		elemCounts[elem.Category]++
	}

	for cat, count := range elemCounts {
		if cat == "container" || cat == "text" {
			continue
		}
		if count > 0 {
			parts = append(parts, strings.ReplaceAll(cat, "_", " ")+"("+itoa(count)+")")
		}
	}

	return strings.Join(parts, ", ")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
