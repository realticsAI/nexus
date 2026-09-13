package model

import "time"

type EdgeType string

const (
	HTTPCall        EdgeType = "HTTP_CALL"
	ExternalCall    EdgeType = "EXTERNAL_CALL"
	KafkaProduce    EdgeType = "KAFKA_PRODUCE"
	KafkaConsume    EdgeType = "KAFKA_CONSUME"
	FrontendAPICall EdgeType = "FRONTEND_API_CALL"
	BuildDependency EdgeType = "BUILD_DEPENDENCY"
	ImportDep       EdgeType = "IMPORT_DEPENDENCY"
)

type Graph struct {
	Version      int              `json:"version"`
	Nodes        []Node           `json:"nodes"`
	Edges        []Edge           `json:"edges"`
	KeywordIndex map[string][]Hit `json:"keyword_index,omitempty"`
	GeneratedAt  time.Time        `json:"generated_at"`
}

type Node struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Platform  string `json:"platform"`
	Workspace string `json:"workspace"`
}

type Edge struct {
	From     string   `json:"from"`
	To       string   `json:"to"`
	Type     EdgeType `json:"type"`
	Evidence string   `json:"evidence,omitempty"`
}

type Hit struct {
	ServiceKey string `json:"service_key"`
	UnitType   string `json:"unit_type"`
	UnitName   string `json:"unit_name"`
	Score      int    `json:"score"`
}
