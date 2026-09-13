package analyzer

import "strings"

const externalNodePrefix = "external:"

type ExternalSystem struct {
	ID           string
	Label        string
	Category     string
	ConfigTokens []string
	HostTokens   []string
}

var externalSystems = []ExternalSystem{
	{ID: "hybris-occ", Label: "SAP Commerce (Hybris OCC)", Category: "commerce-platform",
		ConfigTokens: []string{"hybris.occ", "hybris-secured-webclient", "hybris-commerce-secured-webclient", "hybris.eligibility", "occ.hybris"},
		HostTokens:   []string{"hybris", "occapi"}},
	{ID: "apigee", Label: "Apigee Gateway", Category: "gateway",
		ConfigTokens: []string{"apigee.gateway", "apigee.proxy"},
		HostTokens:   []string{"apigee.net"}},
	{ID: "onesignal", Label: "OneSignal", Category: "notifications",
		ConfigTokens: []string{"onesignal"},
		HostTokens:   []string{"onesignal.com"}},
	{ID: "alipay", Label: "Alipay", Category: "payments",
		ConfigTokens: []string{"alipay"},
		HostTokens:   []string{"alipay.com"}},
	{ID: "solr", Label: "Solr Search", Category: "search",
		ConfigTokens: []string{"solr"},
		HostTokens:   []string{"solr"}},
	{ID: "aem", Label: "Adobe Experience Manager", Category: "content",
		ConfigTokens: []string{"aem.publish", "aem.author", "adobe.aem"},
		HostTokens:   []string{"adobeaemcloud"}},
	{ID: "stripe", Label: "Stripe", Category: "payments",
		ConfigTokens: []string{"stripe"},
		HostTokens:   []string{"stripe.com"}},
	{ID: "twilio", Label: "Twilio", Category: "communications",
		ConfigTokens: []string{"twilio"},
		HostTokens:   []string{"twilio.com"}},
	{ID: "aws-s3", Label: "AWS S3", Category: "storage",
		ConfigTokens: []string{"aws.s3", "s3.bucket"},
		HostTokens:   []string{"s3.amazonaws.com"}},
	{ID: "datadog", Label: "Datadog", Category: "monitoring",
		ConfigTokens: []string{"datadog"},
		HostTokens:   []string{"datadoghq.com"}},
	{ID: "sendgrid", Label: "SendGrid", Category: "email",
		ConfigTokens: []string{"sendgrid"},
		HostTokens:   []string{"sendgrid.com", "sendgrid.net"}},
	{ID: "auth0", Label: "Auth0", Category: "identity",
		ConfigTokens: []string{"auth0"},
		HostTokens:   []string{"auth0.com"}},
	{ID: "firebase", Label: "Firebase", Category: "platform",
		ConfigTokens: []string{"firebase"},
		HostTokens:   []string{"firebase.google.com", "firebaseio.com", "fcm.googleapis.com"}},
	{ID: "elasticsearch", Label: "Elasticsearch", Category: "search",
		ConfigTokens: []string{"elasticsearch", "elastic"},
		HostTokens:   []string{"elastic.co", "es.amazonaws.com"}},
}

// ExternalNodeName returns the graph node key for an external system.
func ExternalNodeName(sys *ExternalSystem) string {
	return externalNodePrefix + sys.ID
}

// IsExternalNode checks whether a node key represents an external system.
func IsExternalNode(name string) bool {
	return strings.HasPrefix(name, externalNodePrefix)
}

// MatchHost matches a URL hostname against the external system registry.
// Returns nil when no system matches.
func MatchHost(hostname string) *ExternalSystem {
	lower := strings.ToLower(hostname)
	for i := range externalSystems {
		for _, token := range externalSystems[i].HostTokens {
			if strings.Contains(lower, token) {
				return &externalSystems[i]
			}
		}
	}
	return nil
}

// MatchConfigRef matches a config key against the registry using
// dot-boundary matching: "app.hybris.occ.url" matches token "hybris.occ"
// because the token sits on dot boundaries within the key.
func MatchConfigRef(ref string) *ExternalSystem {
	lower := strings.ToLower(ref)
	padded := "." + lower + "."
	for i := range externalSystems {
		for _, token := range externalSystems[i].ConfigTokens {
			if strings.Contains(padded, "."+token+".") {
				return &externalSystems[i]
			}
		}
	}
	return nil
}
