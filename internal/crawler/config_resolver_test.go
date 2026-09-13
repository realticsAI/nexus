package crawler

import (
	"testing"
)

func TestResolveConfigPlaceholders_Simple(t *testing.T) {
	props := map[string]string{
		"app.calendar.url": "http://calendar-svc:8080",
	}
	got := ResolveConfigPlaceholders("${app.calendar.url}/api/v1", props)
	want := "http://calendar-svc:8080/api/v1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveConfigPlaceholders_Default(t *testing.T) {
	props := map[string]string{}
	got := ResolveConfigPlaceholders("${SERVICE_URL:http://fallback:8080}", props)
	want := "http://fallback:8080"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveConfigPlaceholders_Chained(t *testing.T) {
	props := map[string]string{
		"customer.service.url": "${CUSTOMER_URL:http://customer:8080}",
	}
	got := ResolveConfigPlaceholders("${customer.service.url}/api", props)
	want := "http://customer:8080/api"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveConfigPlaceholders_Unresolvable(t *testing.T) {
	props := map[string]string{}
	input := "${MISSING_KEY}"
	got := ResolveConfigPlaceholders(input, props)
	if got != input {
		t.Errorf("unresolvable should stay as-is: got %q, want %q", got, input)
	}
}

func TestResolveConfigPlaceholders_Multiple(t *testing.T) {
	props := map[string]string{
		"host": "orders-svc",
		"port": "8080",
	}
	got := ResolveConfigPlaceholders("http://${host}:${port}/api", props)
	want := "http://orders-svc:8080/api"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveAllConfig(t *testing.T) {
	props := map[string]string{
		"base":   "http://api.example.com",
		"app.url": "${base}/v1/orders",
	}
	resolved := ResolveAllConfig(props)
	want := "http://api.example.com/v1/orders"
	if resolved["app.url"] != want {
		t.Errorf("got %q, want %q", resolved["app.url"], want)
	}
}

func TestIsServiceURLKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"app.calendar.url", true},
		{"customer.service.url", true},
		{"external.services.orders.base-url", true},
		{"app.payment.endpoint", true},
		{"app.booking.host", true},
		{"spring.datasource.url", false},
		{"redis.host", false},
		{"kafka.bootstrap-servers", false},
		{"management.endpoint.health", false},
		{"", false},
		{"server.port", false},
		{"app.feature.enabled", false},
	}
	for _, tt := range tests {
		got := IsServiceURLKey(tt.key)
		if got != tt.want {
			t.Errorf("IsServiceURLKey(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestIsResolvableHTTPValue(t *testing.T) {
	tests := []struct {
		val  string
		want bool
	}{
		{"http://orders-svc:8080/api", true},
		{"https://api.example.com/v1", true},
		{"lb://customer-service", true},
		{"orders-svc.default.svc.cluster.local", true},
		{"jdbc:postgresql://db:5432/mydb", false},
		{"redis://cache:6379", false},
		{"mongodb://mongo:27017/test", false},
		{"http://localhost:8080", false},
		{"http://127.0.0.1:8080", false},
		{"${UNRESOLVED}", false},
		{"broker1:9092,broker2:9092", false},
		{"", false},
		{"classpath:schema.graphql", false},
	}
	for _, tt := range tests {
		got := IsResolvableHTTPValue(tt.val)
		if got != tt.want {
			t.Errorf("IsResolvableHTTPValue(%q) = %v, want %v", tt.val, got, tt.want)
		}
	}
}

func TestExtractServiceURLs(t *testing.T) {
	config := map[string]string{
		"app.orders.base-url":       "http://orders-svc:8080/api",
		"customer.service.url":      "${CUSTOMER_HOST:http://customer:8080}/v1",
		"spring.datasource.url":     "jdbc:postgresql://db:5432/mydb",
		"redis.host":                "cache-host",
		"app.feature.enabled":       "true",
		"server.port":               "8080",
		"app.payment.endpoint":      "http://localhost:3000/pay",
		"external.booking.base-url": "https://booking-api.example.com",
	}
	urls := ExtractServiceURLs(config)

	keys := map[string]string{}
	for _, u := range urls {
		keys[u.Key] = u.URL
	}

	if _, ok := keys["app.orders.base-url"]; !ok {
		t.Error("expected app.orders.base-url to be extracted")
	}
	if _, ok := keys["external.booking.base-url"]; !ok {
		t.Error("expected external.booking.base-url to be extracted")
	}
	if _, ok := keys["spring.datasource.url"]; ok {
		t.Error("spring.datasource.url should be filtered out")
	}
	if _, ok := keys["redis.host"]; ok {
		t.Error("redis.host should be filtered out")
	}
	if _, ok := keys["app.payment.endpoint"]; ok {
		t.Error("localhost URL should be filtered out")
	}
	if v, ok := keys["customer.service.url"]; ok {
		if v != "http://customer:8080/v1" {
			t.Errorf("customer URL should be resolved, got %q", v)
		}
	} else {
		t.Error("expected customer.service.url to be extracted with resolved placeholder")
	}
}
