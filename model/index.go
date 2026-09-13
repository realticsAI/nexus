package model

import "time"

type ServiceIndex struct {
	Key           string            `json:"key"`
	Path          string            `json:"path"`
	Platform      Platform          `json:"platform"`
	Units         []Unit            `json:"units"`
	Config        map[string]string `json:"config,omitempty"`
	BuildDeps     []string          `json:"build_deps,omitempty"`
	FileChecksums map[string]string `json:"file_checksums"`
	IndexedAt     time.Time         `json:"indexed_at"`
}

type Unit struct {
	Type          string   `json:"type"`
	Name          string   `json:"name"`
	FQN           string   `json:"fqn,omitempty"`
	File          string   `json:"file"`
	Line          int      `json:"line"`
	Stereotype    string   `json:"stereotype,omitempty"`
	Annotations   []string `json:"annotations,omitempty"`
	Methods       []Method `json:"methods,omitempty"`
	Fields        []Field  `json:"fields,omitempty"`
	Dependencies  []string `json:"dependencies,omitempty"`
	ConfigRefs    []string `json:"config_refs,omitempty"`
	KafkaProduces []string `json:"kafka_produces,omitempty"`
	KafkaConsumes []string `json:"kafka_consumes,omitempty"`
	Endpoints     []string `json:"endpoints,omitempty"`
	ApiCalls      []string `json:"api_calls,omitempty"`
	IsTest        bool     `json:"is_test,omitempty"`
	Imports    []string `json:"imports,omitempty"`
	Extends    string   `json:"extends,omitempty"`
	Implements []string `json:"implements,omitempty"`
	ThrowSites []ThrowSite `json:"throw_sites,omitempty"`
}

type ThrowSite struct {
	ExceptionFQN string   `json:"exception_fqn,omitempty"`
	ErrorCode    string   `json:"error_code,omitempty"`
	Tokens       []string `json:"tokens,omitempty"`
	Method       string   `json:"method,omitempty"`
	Line         int      `json:"line"`
	Kind         string   `json:"kind"`
}

type Field struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Annotations []string `json:"annotations,omitempty"`
}

type MethodCall struct {
	Target string `json:"target"`
	Method string `json:"method"`
}

type Method struct {
	Name        string       `json:"name"`
	Line        int          `json:"line"`
	ReturnType  string       `json:"return_type,omitempty"`
	Parameters  []string     `json:"parameters,omitempty"`
	Annotations []string     `json:"annotations,omitempty"`
	HTTPMethod  string       `json:"http_method,omitempty"`
	HTTPPath    string       `json:"http_path,omitempty"`
	Calls       []MethodCall `json:"calls,omitempty"`
}
