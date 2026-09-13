package crawler

import (
	"testing"
)

func TestResolveKafkaTopic_PlainLiteral(t *testing.T) {
	props := map[string]string{}
	got := ResolveKafkaTopic("order.created", props)
	if len(got) != 1 || got[0] != "order.created" {
		t.Fatalf("expected [order.created], got %v", got)
	}
}

func TestResolveKafkaTopic_SimplePlaceholder(t *testing.T) {
	props := map[string]string{
		"consumer.kafka.topic": "order-events",
	}
	got := ResolveKafkaTopic("${consumer.kafka.topic}", props)
	if len(got) != 1 || got[0] != "order-events" {
		t.Fatalf("expected [order-events], got %v", got)
	}
}

func TestResolveKafkaTopic_PlaceholderWithDefault(t *testing.T) {
	props := map[string]string{}
	got := ResolveKafkaTopic("${consumer.kafka.topic:fallback-topic}", props)
	if len(got) != 1 || got[0] != "fallback-topic" {
		t.Fatalf("expected [fallback-topic], got %v", got)
	}
}

func TestResolveKafkaTopic_PlaceholderWithDefault_KeyPresent(t *testing.T) {
	props := map[string]string{
		"consumer.kafka.topic": "real-topic",
	}
	got := ResolveKafkaTopic("${consumer.kafka.topic:fallback-topic}", props)
	if len(got) != 1 || got[0] != "real-topic" {
		t.Fatalf("expected [real-topic], got %v", got)
	}
}

func TestResolveKafkaTopic_SpELWrapper(t *testing.T) {
	props := map[string]string{
		"kafka.topic.name": "payment-events",
	}
	got := ResolveKafkaTopic("#{'${kafka.topic.name}'}", props)
	if len(got) != 1 || got[0] != "payment-events" {
		t.Fatalf("expected [payment-events], got %v", got)
	}
}

func TestResolveKafkaTopic_SpELWithDefault(t *testing.T) {
	props := map[string]string{}
	got := ResolveKafkaTopic("#{'${kafka.topic.name:spel-default}'}", props)
	if len(got) != 1 || got[0] != "spel-default" {
		t.Fatalf("expected [spel-default], got %v", got)
	}
}

func TestResolveKafkaTopic_SpELUnresolvable(t *testing.T) {
	props := map[string]string{}
	got := ResolveKafkaTopic("#{someBean.getTopics()}", props)
	if got != nil {
		t.Fatalf("expected nil for unresolvable SpEL, got %v", got)
	}
}

func TestResolveKafkaTopic_CommaSeparated(t *testing.T) {
	props := map[string]string{
		"kafka.topics": "topic-a,topic-b,topic-c",
	}
	got := ResolveKafkaTopic("${kafka.topics}", props)
	if len(got) != 3 || got[0] != "topic-a" || got[1] != "topic-b" || got[2] != "topic-c" {
		t.Fatalf("expected [topic-a topic-b topic-c], got %v", got)
	}
}

func TestResolveKafkaTopic_RecursiveResolution(t *testing.T) {
	props := map[string]string{
		"consumer.kafka.topic": "${CONSUMER_KAFKA_TOPIC:real-topic-name}",
	}
	got := ResolveKafkaTopic("${consumer.kafka.topic}", props)
	if len(got) != 1 || got[0] != "real-topic-name" {
		t.Fatalf("expected [real-topic-name], got %v", got)
	}
}

func TestResolveKafkaTopic_RecursiveResolution_EnvPresent(t *testing.T) {
	props := map[string]string{
		"consumer.kafka.topic": "${CONSUMER_KAFKA_TOPIC:default-topic}",
		"CONSUMER_KAFKA_TOPIC": "env-topic",
	}
	got := ResolveKafkaTopic("${consumer.kafka.topic}", props)
	if len(got) != 1 || got[0] != "env-topic" {
		t.Fatalf("expected [env-topic], got %v", got)
	}
}

func TestResolveKafkaTopic_CycleDetection(t *testing.T) {
	props := map[string]string{
		"a": "${b}",
		"b": "${a}",
	}
	got := ResolveKafkaTopic("${a}", props)
	// Should not hang — returns the unresolved placeholder
	if got != nil {
		t.Fatalf("expected nil for cyclic placeholder, got %v", got)
	}
}

func TestResolveKafkaTopic_Empty(t *testing.T) {
	got := ResolveKafkaTopic("", nil)
	if got != nil {
		t.Fatalf("expected nil for empty, got %v", got)
	}
}

func TestResolveKafkaTopic_ShortValue(t *testing.T) {
	got := ResolveKafkaTopic("ab", nil)
	if got != nil {
		t.Fatalf("expected nil for short value, got %v", got)
	}
}

func TestResolveKafkaTopics_Bulk(t *testing.T) {
	props := map[string]string{
		"consumer.kafka.topic": "order-events",
	}
	raw := []string{"literal-topic", "${consumer.kafka.topic}", "another-literal"}
	got := ResolveKafkaTopics(raw, props)
	if len(got) != 3 {
		t.Fatalf("expected 3 topics, got %d: %v", len(got), got)
	}
	if got[0] != "literal-topic" || got[1] != "order-events" || got[2] != "another-literal" {
		t.Fatalf("unexpected result: %v", got)
	}
}

func TestResolveKafkaTopics_EmptyProps(t *testing.T) {
	raw := []string{"${unresolvable}"}
	got := ResolveKafkaTopics(raw, nil)
	if len(got) != 1 || got[0] != "${unresolvable}" {
		t.Fatalf("expected raw passthrough with nil props, got %v", got)
	}
}

func TestResolveKafkaTopics_NilInput(t *testing.T) {
	got := ResolveKafkaTopics(nil, map[string]string{"a": "b"})
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestIsCleanTopic(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"order-events", true},
		{"com.acme.loyalty.events", true},
		{"topic_name_v2", true},
		{"", false},
		{"ab", false},
		{"${unresolved}", false},
		{"true", false},
		{"false", false},
		{"12345", false},
		{"has space", false},
		{"has/slash", false},
		{"has:colon", false},
	}
	for _, tc := range cases {
		got := isCleanTopic(tc.input)
		if got != tc.want {
			t.Errorf("isCleanTopic(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}
