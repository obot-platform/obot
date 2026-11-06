package llmengine

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
)

func TestAuthenticationSwapPersistsAcrossRestart(t *testing.T) {
	// Azure server identity requires Azure hosts; resolve fixtures without external DNS.
	previousResolver := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: testKeyDNS}
	t.Cleanup(func() { net.DefaultResolver = previousResolver })
	e, ctx := startTestEngine(t)
	for _, provider := range []string{"azure", "bedrock"} {
		if _, err := e.CreateProvider(ctx, mustJSON(t, map[string]any{"provider": provider})); err != nil {
			t.Fatal(err)
		}
		for _, from := range []string{"api", "credentials", "server"} {
			for _, to := range []string{"api", "credentials", "server"} {
				if from == to {
					continue
				}
				t.Run(provider+"/"+from+"_to_"+to, func(t *testing.T) {
					configName := provider + "_key_config"
					sourceValue, sourceConfig := swapTestCredentials(provider, from, "source")
					targetValue, targetConfig := swapTestCredentials(provider, to, "target")
					name := provider + "-" + from + "-to-" + to
					payload := func(value string, config map[string]string) []byte {
						return mustJSON(t, map[string]any{
							"name":   name,
							"value":  value,
							"models": []string{"*"},
							"weight": 1,
							// Disabled keys exercise persistence and reconciliation without cloud calls.
							"enabled":  false,
							configName: config,
						})
					}
					// CreateKey and UpdateKey run GuardProviderChange inside the resource mutation.
					created, err := e.CreateKey(ctx, provider, payload(sourceValue, sourceConfig))
					if err != nil {
						t.Fatal(err)
					}
					if created.ID == "" {
						t.Fatal("created key has no ID")
					}

					assertSaved := func(engine *Engine, phase, wantValue string, wantConfig map[string]string) {
						t.Helper()
						persisted, err := engine.store.GetProvider(ctx, provider)
						if err != nil {
							t.Fatalf("%s: read stored provider: %v", phase, err)
						}
						index := slices.IndexFunc(persisted.Keys, func(key schemas.Key) bool { return key.ID == created.ID })
						if index < 0 {
							t.Fatalf("%s: stored key is missing", phase)
						}
						raw := mustJSON(t, persisted.Keys[index])
						response, err := engine.GetKey(ctx, provider, created.ID)
						if err != nil {
							t.Fatalf("%s: read key through admin API: %v", phase, err)
						}
						listed := mustJSON(t, response)
						want := map[string]string{"value": wantValue}
						for field, value := range wantConfig {
							want[configName+"."+field] = value
						}
						for field, value := range want {
							at := strings.Split(field, ".")
							stored, _ := valueAt(raw, at)
							if SecretValue(stored) != value {
								t.Errorf("%s: raw %s did not match selected configuration", phase, field)
							}
							exposed, _ := valueAt(listed, at)
							if value == "" {
								if HasSecretValue(exposed) {
									t.Errorf("%s: API still reports a value for cleared %s", phase, field)
								}
							} else if !HasSecretValue(exposed) {
								t.Errorf("%s: API lost selected %s", phase, field)
							}
						}
					}

					// Establish that the old credentials were actually stored, rather than
					// passing the removal assertions because the key started empty.
					assertSaved(e, "before swap", sourceValue, sourceConfig)
					if _, err := e.UpdateKey(ctx, provider, created.ID, payload(targetValue, targetConfig)); err != nil {
						t.Fatal(err)
					}
					assertSaved(e, "after swap", targetValue, targetConfig)
					if _, err := e.Sync(ctx); err != nil {
						t.Fatal(err)
					}
					assertSaved(e, "after reconciliation", targetValue, targetConfig)

					// A new replica must reconstruct the selected authentication from resources.
					restarted := New()
					if err := restarted.Start(ctx, e.opts); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() {
						if err := restarted.Shutdown(context.Background()); err != nil {
							t.Error(err)
						}
					})
					assertSaved(restarted, "after restart", targetValue, targetConfig)
				})
			}
		}
	}
}

// Build all credential fields, including explicit blanks, as the key dialog does.
// Distinct source/target values also catch accidentally retaining a selected field's old value.
func swapTestCredentials(provider, method, prefix string) (string, map[string]string) {
	value := ""
	var config map[string]string
	if provider == "azure" {
		endpoint := "https://source.openai.azure.com"
		if prefix == "target" {
			endpoint = "https://target.openai.azure.com"
		}
		config = map[string]string{
			"endpoint":      endpoint,
			"client_id":     "",
			"client_secret": "",
			"tenant_id":     "",
		}
		if method == "credentials" {
			config["client_id"] = prefix + "-client-id"
			config["client_secret"] = prefix + "-client-secret"
			config["tenant_id"] = prefix + "-tenant-id"
		}
	} else {
		config = map[string]string{
			"region":        "us-east-1",
			"access_key":    "",
			"secret_key":    "",
			"session_token": "",
			"role_arn":      "",
		}
		if method == "credentials" {
			config["access_key"] = prefix + "-access-key"
			config["secret_key"] = prefix + "-secret-key"
			config["session_token"] = prefix + "-session-token"
		}
		if method != "api" {
			config["role_arn"] = fmt.Sprintf("arn:aws:iam::123456789012:role/%s-%s", prefix, method)
		}
	}
	if method == "api" {
		value = prefix + "-api-key"
	}
	return value, config
}
