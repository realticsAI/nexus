package crawler

import (
	"regexp"
	"strings"
)

var placeholderRe = regexp.MustCompile(`\$\{([^${}]+(?::[^${}]+)?)\}`)

// ResolveConfigPlaceholders resolves ${key} and ${key:default} placeholders
// in a value string against a properties map. Recurses up to maxDepth levels
// to handle chained placeholders. Cycle detection via a seen-set per pass.
func ResolveConfigPlaceholders(value string, props map[string]string) string {
	return resolvePlaceholders(value, props, 0)
}

const maxResolveDepth = 8

func resolvePlaceholders(input string, props map[string]string, depth int) string {
	if depth >= maxResolveDepth || !strings.Contains(input, "${") {
		return input
	}

	result := placeholderRe.ReplaceAllStringFunc(input, func(match string) string {
		inner := match[2 : len(match)-1]
		key, defaultVal := inner, ""
		if colonIdx := strings.IndexByte(inner, ':'); colonIdx >= 0 {
			key = inner[:colonIdx]
			defaultVal = inner[colonIdx+1:]
		}
		key = strings.TrimSpace(key)

		if val, ok := props[key]; ok {
			return val
		}
		if defaultVal != "" {
			return defaultVal
		}
		return match
	})

	if result == input {
		return result
	}
	return resolvePlaceholders(result, props, depth+1)
}

// ResolveAllConfig resolves all placeholder values in a config map in-place
// and returns the resolved map.
func ResolveAllConfig(props map[string]string) map[string]string {
	resolved := make(map[string]string, len(props))
	for k, v := range props {
		resolved[k] = ResolveConfigPlaceholders(v, props)
	}
	return resolved
}

var infraURLRoots = map[string]bool{
	"spring": true, "management": true, "server": true, "logging": true,
	"actuator": true, "datasource": true, "redis": true, "couchbase": true,
	"mongo": true, "mongodb": true, "cassandra": true, "elasticsearch": true,
	"kafka": true, "sqs": true, "sns": true, "rabbit": true, "rabbitmq": true,
	"zookeeper": true, "eureka": true, "consul": true, "vault": true,
	"zipkin": true, "jaeger": true, "otel": true, "opentelemetry": true,
	"hikari": true, "jpa": true, "hibernate": true, "resilience4j": true,
	"prometheus": true, "grafana": true,
}

var serviceURLSuffixes = map[string]bool{
	"url": true, "uri": true, "endpoint": true, "host": true,
	"hostname": true, "address": true, "base-url": true, "baseurl": true,
	"base-uri": true, "basepath": true, "base-path": true,
}

// IsServiceURLKey returns true if a config key looks like it holds a
// downstream service URL (ends in .url, .uri, .endpoint, .host, etc.)
// and doesn't belong to an infrastructure root (spring.*, redis.*, etc.).
func IsServiceURLKey(key string) bool {
	if key == "" {
		return false
	}
	lower := strings.ToLower(key)
	root := lower
	if dot := strings.IndexByte(lower, '.'); dot >= 0 {
		root = lower[:dot]
	}
	if infraURLRoots[root] {
		return false
	}
	last := lower
	if dot := strings.LastIndexByte(lower, '.'); dot >= 0 {
		last = lower[dot+1:]
	}
	if serviceURLSuffixes[last] {
		return true
	}
	if strings.HasSuffix(lower, ".base.url") || strings.HasSuffix(lower, ".service.url") {
		return true
	}
	return false
}

// IsResolvableHTTPValue returns true if a config value looks like a
// reachable HTTP URL (not infra connection strings, localhost, or
// unresolved placeholders).
func IsResolvableHTTPValue(value string) bool {
	if value == "" {
		return false
	}
	v := strings.TrimSpace(value)
	if strings.Contains(v, "${") || strings.Contains(v, "#{") {
		return false
	}
	lower := strings.ToLower(v)
	for _, prefix := range []string{"jdbc:", "redis:", "mongodb:", "amqp:", "kafka:", "file:", "classpath:"} {
		if strings.HasPrefix(lower, prefix) {
			return false
		}
	}
	if strings.Contains(lower, "localhost") || strings.Contains(lower, "127.0.0.1") || strings.Contains(lower, "0.0.0.0") {
		return false
	}
	if strings.Contains(v, ",") {
		return false
	}
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "lb://") ||
		(strings.Contains(v, ".") && !strings.Contains(v, " ") && !strings.HasPrefix(v, "/"))
}

// ExtractServiceURLs returns (configKey, resolvedURL) pairs from a config
// map that look like downstream service URLs after placeholder resolution.
func ExtractServiceURLs(config map[string]string) []ConfigURL {
	resolved := ResolveAllConfig(config)
	var urls []ConfigURL
	for key, val := range resolved {
		if !IsServiceURLKey(key) {
			continue
		}
		if !IsResolvableHTTPValue(val) {
			continue
		}
		urls = append(urls, ConfigURL{Key: key, URL: val})
	}
	return urls
}

type ConfigURL struct {
	Key string
	URL string
}
