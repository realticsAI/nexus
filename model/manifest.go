package model

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

type Manifest struct {
	Version     int            `json:"version"`
	Services    []ServiceEntry `json:"services"`
	GeneratedAt time.Time      `json:"generated_at"`
}

type ServiceEntry struct {
	Key            string         `json:"key"`
	Path           string         `json:"path"`
	Platform       string         `json:"platform"`
	HeadSHA        string         `json:"head_sha"`
	FileCount      int            `json:"file_count"`
	UnitCount      int            `json:"unit_count"`
	EndpointCount  int            `json:"endpoint_count"`
	ConfigKeyCount int            `json:"config_key_count"`
	Stereotypes    map[string]int `json:"stereotypes,omitempty"`
	BuildDeps      []string       `json:"build_deps,omitempty"`
	Endpoints      []string       `json:"endpoints,omitempty"`
	Keywords       []string       `json:"keywords,omitempty"`
	LastIndexed    string         `json:"last_indexed"`
}

type ServiceSummary struct {
	Key            string         `json:"key"`
	Path           string         `json:"path"`
	Platform       Platform       `json:"platform"`
	UnitCount      int            `json:"unit_count"`
	EndpointCount  int            `json:"endpoint_count"`
	ConfigKeyCount int            `json:"config_key_count"`
	Stereotypes    map[string]int `json:"stereotypes,omitempty"`
	BuildDeps      []string       `json:"build_deps,omitempty"`
	Endpoints      []string       `json:"endpoints,omitempty"`
	Keywords       []string       `json:"keywords,omitempty"`
	IndexedAt      time.Time      `json:"indexed_at"`
}

var modelDebug = os.Getenv("NEXUS_DEBUG") == "1" || strings.EqualFold(os.Getenv("NEXUS_DEBUG"), "true")

func BuildServiceSummary(idx *ServiceIndex) *ServiceSummary {
	t := time.Now()
	if modelDebug {
		fmt.Fprintf(os.Stderr, "[DEBUG:model] BuildServiceSummary key=%s units=%d\n", idx.Key, len(idx.Units))
	}
	s := &ServiceSummary{
		Key:            idx.Key,
		Path:           idx.Path,
		Platform:       idx.Platform,
		UnitCount:      len(idx.Units),
		ConfigKeyCount: len(idx.Config),
		Stereotypes:    make(map[string]int),
		BuildDeps:      idx.BuildDeps,
		IndexedAt:      idx.IndexedAt,
	}

	keywordSet := map[string]bool{}

	for _, u := range idx.Units {
		if u.Stereotype != "" {
			s.Stereotypes[u.Stereotype]++
		}
		for _, ep := range u.Endpoints {
			s.Endpoints = append(s.Endpoints, ep)
			addPathKeywords(ep, keywordSet)
		}
		addCamelCaseKeywords(u.Name, keywordSet)
	}
	s.EndpointCount = len(s.Endpoints)

	for configKey, configVal := range idx.Config {
		keyLower := strings.ToLower(configKey)
		if strings.Contains(keyLower, "domain") || strings.Contains(keyLower, "url") || strings.Contains(keyLower, "host") {
			addURLKeywords(configVal, keywordSet)
		}
	}

	addCamelCaseKeywords(idx.Key, keywordSet)

	for kw := range keywordSet {
		if len(kw) > 2 {
			s.Keywords = append(s.Keywords, kw)
		}
	}
	if len(s.Keywords) > 50 {
		s.Keywords = s.Keywords[:50]
	}

	if modelDebug {
		fmt.Fprintf(os.Stderr, "[DEBUG:model] BuildServiceSummary key=%s → keywords=%d endpoints=%d stereotypes=%d took=%s\n",
			idx.Key, len(s.Keywords), len(s.Endpoints), len(s.Stereotypes), time.Since(t).Round(time.Millisecond))
	}
	return s
}

var camelSplitRe = regexp.MustCompile(`[A-Z][a-z]+|[a-z]+|[A-Z]{2,}`)

func addCamelCaseKeywords(name string, set map[string]bool) {
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, ".", " ")
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, ":", " ")

	for _, word := range strings.Fields(name) {
		set[strings.ToLower(word)] = true
		for _, part := range camelSplitRe.FindAllString(word, -1) {
			lower := strings.ToLower(part)
			if lower != "" {
				set[lower] = true
			}
		}
	}
}

func addPathKeywords(endpoint string, set map[string]bool) {
	parts := strings.Fields(endpoint)
	for _, p := range parts {
		for _, seg := range strings.Split(p, "/") {
			seg = strings.TrimSpace(seg)
			if seg == "" || strings.HasPrefix(seg, "{") || strings.HasPrefix(seg, "$") {
				continue
			}
			set[strings.ToLower(seg)] = true
		}
	}
}

func addURLKeywords(val string, set map[string]bool) {
	val = strings.TrimSpace(val)
	for _, prefix := range []string{"https://", "http://", "${"} {
		val = strings.TrimPrefix(val, prefix)
	}
	val = strings.TrimRight(val, "}")

	for _, seg := range strings.FieldsFunc(val, func(r rune) bool {
		return r == '/' || r == '.' || r == ':' || r == '-' || r == '_'
	}) {
		lower := strings.ToLower(seg)
		skip := []string{"com", "io", "api", "acme", "private", "dev", "ecom", "www", "https", "http", "v1", "v2", "v3"}
		isSkip := false
		for _, s := range skip {
			if lower == s {
				isSkip = true
				break
			}
		}
		if !isSkip && len(lower) > 2 {
			set[lower] = true
		}
	}
}
