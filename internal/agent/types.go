package agent

import (
	"github.com/anurag/nexus/internal/figma"
	"github.com/anurag/nexus/internal/jira"
	"github.com/anurag/nexus/model"
)

type TicketType string

const (
	Bug      TicketType = "BUG"
	Feature  TicketType = "FEATURE"
	Refactor TicketType = "REFACTOR"
	Chore    TicketType = "CHORE"
	Prep     TicketType = "PREP"
)

type WorkPhase string

const (
	PhaseUnderstand  WorkPhase = "UNDERSTAND"
	PhaseInvestigate WorkPhase = "INVESTIGATE"
	PhaseAnalyze     WorkPhase = "ANALYZE"
	PhasePlan        WorkPhase = "PLAN"
	PhaseDesign      WorkPhase = "DESIGN"
	PhaseExecute     WorkPhase = "EXECUTE"
	PhaseValidate    WorkPhase = "VALIDATE"
	PhaseReplan      WorkPhase = "REPLAN"
	PhaseReview      WorkPhase = "REVIEW"
	PhaseVerify      WorkPhase = "VERIFY"
	PhaseShip        WorkPhase = "SHIP"
)

type WorkResult struct {
	TicketKey     string                `json:"ticket_key"`
	TicketType    TicketType            `json:"ticket_type"`
	Branch        string                `json:"branch"`
	PRUrl         string                `json:"pr_url,omitempty"`
	FilesChanged  []string              `json:"files_changed"`
	TokensUsed    int                   `json:"tokens_used"`
	Phase         WorkPhase             `json:"phase"`
	Error         string                `json:"error,omitempty"`
	DryRun        bool                  `json:"dry_run"`
	Investigation *InvestigationContext  `json:"investigation,omitempty"`
	Analysis      *AnalysisResult       `json:"analysis,omitempty"`
	Plan            *Plan                 `json:"plan,omitempty"`
	Validation      *ValidationResult     `json:"validation,omitempty"`
	TechnicalDesign  string                `json:"technical_design,omitempty"`
	TDConfluenceURL  string                `json:"td_confluence_url,omitempty"`
}

type TicketPlatform string

const (
	PlatformRN      TicketPlatform = "react-native"
	PlatformIOS     TicketPlatform = "ios"
	PlatformAndroid TicketPlatform = "android"
	PlatformWeb     TicketPlatform = "web"
	PlatformBackend TicketPlatform = "backend"
	PlatformUnknown TicketPlatform = ""
)

type InvestigationContext struct {
	TicketPlatform    TicketPlatform       `json:"ticket_platform,omitempty"`
	PlatformRepoFound bool                 `json:"platform_repo_found"`
	Warnings          []string             `json:"warnings,omitempty"`
	Services          []string             `json:"services"`
	RepoPath          string               `json:"repo_path,omitempty"`
	ServicePaths      map[string]string    `json:"service_paths,omitempty"`
	Endpoints         []string             `json:"endpoints"`
	Dependencies      []string             `json:"dependencies"`
	RelatedFiles      []string             `json:"related_files"`
	RelatedClasses    []ClassDetail        `json:"related_classes,omitempty"`
	CallChain         []CallChainLink      `json:"call_chain,omitempty"`
	ExistingFiles     []string             `json:"existing_files,omitempty"`
	ExistingEndpoints []EndpointDetail     `json:"existing_endpoints,omitempty"`
	ExternalAPIs      []ExternalAPI        `json:"external_apis,omitempty"`
	FileExcerpts      []FileExcerpt        `json:"file_excerpts,omitempty"`
	FigmaDesigns      []FigmaDesignRef     `json:"figma_designs,omitempty"`
	LinkedIssues      []LinkedIssueSummary `json:"linked_issues,omitempty"`
	Patterns          []string             `json:"patterns,omitempty"`
	BackendAPIs       []EndpointDetail     `json:"backend_apis,omitempty"`
	LibraryAPIs       []LibraryAPI         `json:"library_apis,omitempty"`
}

type LibraryAPI struct {
	GroupID    string `json:"group_id"`
	ArtifactID string `json:"artifact_id"`
	Version    string `json:"version"`
	Classes    []LibraryClass `json:"classes"`
}

type LibraryClass struct {
	Name      string   `json:"name"`
	Package   string   `json:"package"`
	Fields    []string `json:"fields"`
	Methods   []string `json:"methods"`
	IsBuilder bool     `json:"is_builder,omitempty"`
}

type LinkedIssueSummary struct {
	Key      string         `json:"key"`
	Summary  string         `json:"summary"`
	Status   string         `json:"status"`
	Type     string         `json:"type"`
	Assignee string         `json:"assignee,omitempty"`
	Comments []jira.Comment `json:"comments,omitempty"`
}

type EndpointDetail struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Class  string `json:"class"`
	File   string `json:"file"`
}

type ExternalAPI struct {
	FacadeClass string   `json:"facade_class"`
	File        string   `json:"file"`
	URLs        []string `json:"urls,omitempty"`
	ConfigKeys  []string `json:"config_keys,omitempty"`
	Internal    bool     `json:"internal"`
}

type ClassDetail struct {
	Name         string       `json:"name"`
	File         string       `json:"file"`
	Service      string       `json:"service"`
	Stereotype   string       `json:"stereotype"`
	Annotations  []string     `json:"annotations,omitempty"`
	Methods      []string     `json:"methods,omitempty"`
	Endpoints    []string     `json:"endpoints,omitempty"`
	Headers      []string     `json:"headers,omitempty"`
	Dependencies []string     `json:"dependencies,omitempty"`
	Imports      []string     `json:"imports,omitempty"`
	Fields       []model.Field `json:"fields,omitempty"`
}

type CallChainLink struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Via      string `json:"via"`
}

type FileExcerpt struct {
	File      string `json:"file"`
	ClassName string `json:"class_name"`
	Signature string `json:"signature"`
}

type FigmaDesignRef struct {
	URL            string                      `json:"url"`
	FileKey        string                      `json:"file_key"`
	NodeID         string                      `json:"node_id,omitempty"`
	FileName       string                      `json:"file_name"`
	Pages          []string                    `json:"pages,omitempty"`
	Components     []string                    `json:"components,omitempty"`
	Frames         []string                    `json:"frames,omitempty"`
	UIText         []string                    `json:"ui_text,omitempty"`
	ScreenStates   []string                    `json:"screen_states,omitempty"`
	ScreenAnalysis *figma.ScreenClassification `json:"screen_analysis,omitempty"`
}

type AnalysisResult struct {
	Requirements []Requirement `json:"requirements"`
	EdgeCases    []EdgeCase    `json:"edge_cases"`
	Unknowns     []string      `json:"unknowns,omitempty"`
}

type Requirement struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Category    string `json:"category"`
	CodeImpact  string `json:"code_impact"`
	Priority    string `json:"priority"`
}

type EdgeCase struct {
	Trigger    string `json:"trigger"`
	Expected   string `json:"expected_behavior"`
	CodeImpact string `json:"code_impact"`
}

type ValidationResult struct {
	Verdict         string              `json:"verdict"`
	Dimensions      []ValidationDimension `json:"dimensions"`
	Risks           []string            `json:"risks,omitempty"`
	Suggestions     []string            `json:"suggestions,omitempty"`
	MissingFromPlan []string            `json:"missing_from_plan,omitempty"`
}

type ValidationDimension struct {
	Name    string `json:"name"`
	Verdict string `json:"verdict"`
	Detail  string `json:"detail"`
}

type Plan struct {
	Summary string     `json:"summary"`
	Steps   []PlanStep `json:"steps"`
}

type PlanStep struct {
	Action  string   `json:"action"`
	File    string   `json:"file"`
	Service string   `json:"service,omitempty"`
	Content string   `json:"content"`
	Covers  []string `json:"covers,omitempty"`
}
