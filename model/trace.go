package model

import "time"

type Confidence int

const (
	High Confidence = iota
	Medium
	Low
)

func (c Confidence) String() string {
	switch c {
	case High:
		return "HIGH"
	case Medium:
		return "MEDIUM"
	default:
		return "LOW"
	}
}

type TraceContext struct {
	Confidence       Confidence `json:"confidence"`
	ServicesConsulted []string   `json:"services_consulted"`
	DataSources      []string   `json:"data_sources"`
	IndexAge         string     `json:"index_age"`
	StaleServices    []string   `json:"stale_services,omitempty"`
	IndexTimestamp   time.Time  `json:"index_timestamp"`
}
