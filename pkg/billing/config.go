package billing

import (
	"strings"
)

// Config points Obot at the billing service that owns this environment's
// subscription.
type Config struct {
	BillingURL string `usage:"Billing service base URL; with a billing key, the subscription supplies seat, hosted MCP server and audit retention limits" name:"billing-url" env:"OBOT_SERVER_BILLING_URL"`
	BillingKey string `usage:"Billing key identifying this environment to the billing service" name:"billing-key" env:"OBOT_SERVER_BILLING_KEY"`
}

// Configured reports whether both billing settings are present. Billing is off
// unless they are, and the license key supplies every limit as it does today.
func (c Config) Configured() bool {
	return strings.TrimSpace(c.BillingURL) != "" && strings.TrimSpace(c.BillingKey) != ""
}
