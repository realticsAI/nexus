package model

import "time"

type CrawlHealth struct {
	TotalRepos    int           `json:"total_repos"`
	Indexed       int           `json:"indexed"`
	Failed        []RepoFailure `json:"failed,omitempty"`
	CrawlDuration string        `json:"crawl_duration"`
	Timestamp     time.Time     `json:"timestamp"`
}

type RepoFailure struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Reason string `json:"reason"`
}
