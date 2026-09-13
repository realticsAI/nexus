package crawler

import (
	"regexp"
	"strings"

	"github.com/anurag/nexus/model"
)

var spelInnerRe = regexp.MustCompile(`\$\{[^}]+}`)

// ResolveKafkaTopic resolves a raw Kafka topic string (from @KafkaListener or
// kafkaTemplate.send) against the service's config properties. Handles:
//   - Plain literals: "order.created" → ["order.created"]
//   - ${key} placeholders: "${consumer.kafka.topic}" → resolved from props
//   - ${key:default} defaults: "${topic:order-events}" → "order-events" if key missing
//   - SpEL wrappers: "#{'${foo}'}" → extracts inner ${} and resolves
//   - Comma-separated values: split into multiple topics
//
// Returns nil if the topic can't be resolved to any clean value.
func ResolveKafkaTopic(raw string, props map[string]string) []string {
	if raw == "" {
		return nil
	}

	candidate := raw

	// Strip SpEL wrappers: "#{'${foo}'}" or "#{'${foo:bar}'}" → "${foo:bar}"
	if strings.HasPrefix(candidate, "#{") {
		match := spelInnerRe.FindString(candidate)
		if match == "" {
			return nil
		}
		candidate = match
	}

	if strings.Contains(candidate, "${") {
		candidate = ResolveConfigPlaceholders(candidate, props)
	}

	return splitCleanTopics(candidate)
}

// ResolveKafkaTopics resolves a slice of raw topic strings, returning all
// resolved topics flattened into a single slice.
func ResolveKafkaTopics(rawTopics []string, props map[string]string) []string {
	if len(rawTopics) == 0 || len(props) == 0 {
		return rawTopics
	}

	var resolved []string
	for _, raw := range rawTopics {
		if !strings.Contains(raw, "${") && !strings.HasPrefix(raw, "#{") {
			resolved = append(resolved, raw)
			continue
		}
		topics := ResolveKafkaTopic(raw, props)
		if len(topics) > 0 {
			resolved = append(resolved, topics...)
		} else {
			resolved = append(resolved, raw)
		}
	}
	return resolved
}

func splitCleanTopics(value string) []string {
	if value == "" {
		return nil
	}
	if !strings.Contains(value, ",") {
		if isCleanTopic(value) {
			return []string{strings.TrimSpace(value)}
		}
		return nil
	}
	var out []string
	for _, part := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(part)
		if isCleanTopic(trimmed) {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// resolveKafkaTopicsInUnits resolves Kafka topic placeholders in all units
// against the service's config properties. Called after parsing + signal
// collection, when both units and config are available.
func resolveKafkaTopicsInUnits(units []model.Unit, config map[string]string) {
	if len(config) == 0 {
		return
	}
	for i := range units {
		units[i].KafkaConsumes = ResolveKafkaTopics(units[i].KafkaConsumes, config)
		units[i].KafkaProduces = ResolveKafkaTopics(units[i].KafkaProduces, config)
	}
}

func isCleanTopic(v string) bool {
	if v == "" || len(v) < 3 {
		return false
	}
	if strings.Contains(v, "${") {
		return false
	}
	for _, bad := range []string{"/", ":", " ", "=", ",", ";"} {
		if strings.Contains(v, bad) {
			return false
		}
	}
	lower := strings.ToLower(v)
	if lower == "true" || lower == "false" {
		return false
	}
	for _, c := range v {
		if c < '0' || c > '9' {
			return true
		}
	}
	return false
}
