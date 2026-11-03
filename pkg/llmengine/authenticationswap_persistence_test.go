package llmengine

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
)

func TestAuthenticationSwapPersistsAcrossReload(t *testing.T) {
	e, ctx := startTestEngine(t)
	var revision int64
	for _, provider := range []string{"azure", "bedrock"} {
		if err := e.Do(ctx, http.MethodPost, "/api/providers", map[string]any{"provider": provider}, nil); err != nil {
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
							// Disabled keys use the real persistence path without cloud token
							// acquisition or model discovery during writes and reloads.
							"enabled":  false,
							configName: config,
						})
					}
					createPath := "providers/" + provider + "/keys"
					change, err := e.GuardProviderChange(ctx, http.MethodPost, createPath, payload(sourceValue, sourceConfig), false)
					if err != nil {
						t.Fatal(err)
					}
					var created schemas.Key
					if err := e.Do(ctx, http.MethodPost, "/api/"+createPath, json.RawMessage(change.Body), &created); err != nil {
						t.Fatal(err)
					}
					if created.ID == "" {
						t.Fatal("created key has no ID")
					}
					path := createPath + "/" + created.ID

					assertSaved := func(phase, wantValue string, wantConfig map[string]string) {
						t.Helper()
						rawKey, err := e.inst.Load().server.Config.GetProviderKeyRaw(schemas.ModelProvider(provider), created.ID)
						if err != nil {
							t.Fatalf("%s: read raw key: %v", phase, err)
						}
						raw := mustJSON(t, rawKey)
						var response map[string]any
						if err := e.Do(ctx, http.MethodGet, "/api/"+path, nil, &response); err != nil {
							t.Fatalf("%s: read key through API: %v", phase, err)
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
					assertSaved("before swap", sourceValue, sourceConfig)
					change, err = e.GuardProviderChange(ctx, http.MethodPut, path, payload(targetValue, targetConfig), false)
					if err != nil {
						t.Fatal(err)
					}
					if err := e.Do(ctx, http.MethodPut, "/api/"+path, json.RawMessage(change.Body), nil); err != nil {
						t.Fatal(err)
					}
					assertSaved("after swap", targetValue, targetConfig)

					revision++
					if reloaded, err := e.Reload(revision); err != nil || !reloaded {
						t.Fatalf("reload = %v, error = %v", reloaded, err)
					}
					assertSaved("after reload", targetValue, targetConfig)
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
		endpoint := "https://8.8.8.8"
		if prefix == "target" {
			endpoint = "https://8.8.4.4"
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
