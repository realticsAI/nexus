package analyzer

import "testing"

func TestMatchHost(t *testing.T) {
	tests := []struct {
		hostname string
		wantID   string
	}{
		{"api.onesignal.com", "onesignal"},
		{"onesignal.com", "onesignal"},
		{"commerce-hybris-proxy.dcx.svc.cluster.local", "hybris-occ"},
		{"occapi.example.com", "hybris-occ"},
		{"gateway.apigee.net", "apigee"},
		{"api.stripe.com", "stripe"},
		{"api.twilio.com", "twilio"},
		{"my-bucket.s3.amazonaws.com", "aws-s3"},
		{"api.datadoghq.com", "datadog"},
		{"api.sendgrid.com", "sendgrid"},
		{"myapp.auth0.com", "auth0"},
		{"fcm.googleapis.com", "firebase"},
		{"search.elastic.co", "elasticsearch"},
		{"internal-service.example.com", ""},
		{"localhost:8080", ""},
		{"", ""},
	}
	for _, tt := range tests {
		sys := MatchHost(tt.hostname)
		if tt.wantID == "" {
			if sys != nil {
				t.Errorf("MatchHost(%q) = %q, want nil", tt.hostname, sys.ID)
			}
		} else {
			if sys == nil {
				t.Errorf("MatchHost(%q) = nil, want %q", tt.hostname, tt.wantID)
			} else if sys.ID != tt.wantID {
				t.Errorf("MatchHost(%q) = %q, want %q", tt.hostname, sys.ID, tt.wantID)
			}
		}
	}
}

func TestMatchConfigRef(t *testing.T) {
	tests := []struct {
		ref    string
		wantID string
	}{
		{"app.hybris.occ.url", "hybris-occ"},
		{"environment.hybris-secured-webclient.url", "hybris-occ"},
		{"app.onesignal.api-key", "onesignal"},
		{"external.services.apigee.gateway.url", "apigee"},
		{"stripe.api-key", "stripe"},
		{"app.twilio.sid", "twilio"},
		{"aws.s3.bucket-name", "aws-s3"},
		{"spring.datasource.url", ""},
		{"app.my-internal-service.url", ""},
		{"", ""},
	}
	for _, tt := range tests {
		sys := MatchConfigRef(tt.ref)
		if tt.wantID == "" {
			if sys != nil {
				t.Errorf("MatchConfigRef(%q) = %q, want nil", tt.ref, sys.ID)
			}
		} else {
			if sys == nil {
				t.Errorf("MatchConfigRef(%q) = nil, want %q", tt.ref, tt.wantID)
			} else if sys.ID != tt.wantID {
				t.Errorf("MatchConfigRef(%q) = %q, want %q", tt.ref, sys.ID, tt.wantID)
			}
		}
	}
}

func TestExternalNodeName(t *testing.T) {
	sys := &ExternalSystem{ID: "hybris-occ"}
	got := ExternalNodeName(sys)
	if got != "external:hybris-occ" {
		t.Errorf("ExternalNodeName = %q, want %q", got, "external:hybris-occ")
	}
}

func TestIsExternalNode(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"external:hybris-occ", true},
		{"external:stripe", true},
		{"backend:commerce.cart", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsExternalNode(tt.name); got != tt.want {
			t.Errorf("IsExternalNode(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestMatchHostCaseInsensitive(t *testing.T) {
	sys := MatchHost("API.STRIPE.COM")
	if sys == nil || sys.ID != "stripe" {
		t.Errorf("MatchHost should be case-insensitive, got %v", sys)
	}
}

func TestMatchConfigRefDotBoundary(t *testing.T) {
	if sys := MatchConfigRef("mysolrconfig.url"); sys != nil {
		t.Errorf("MatchConfigRef(%q) should not match without dot boundary, got %q", "mysolrconfig.url", sys.ID)
	}
	if sys := MatchConfigRef("my.solr.url"); sys == nil || sys.ID != "solr" {
		t.Errorf("MatchConfigRef(%q) should match solr on dot boundary", "my.solr.url")
	}
}
