package dispatcher

import (
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestDaemonEnvInheritsNetworkSettings(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses the env command")
	}
	t.Setenv("HTTPS_PROXY", "http://proxy.internal:3128")
	t.Setenv("no_proxy", "localhost,127.0.0.1")
	t.Setenv("SSL_CERT_FILE", "/etc/obot/ca.pem")
	t.Setenv("SSL_CERT_DIR", "/etc/obot/certs")
	// The providers are Go programs, which don't read Node.js's CA variable.
	t.Setenv("NODE_EXTRA_CA_CERTS", "/etc/obot/ca.pem")
	t.Setenv("OBOT_SECRET_SETTING", "not for providers")

	cmd := exec.Command("env")
	cmd.Env = daemonEnv(map[string]string{
		"OBOT_AUTH_PROVIDER_EMAIL_DOMAINS": "*",
		// A provider's own configuration wins over the inherited value.
		"SSL_CERT_DIR": "/provider/certs",
	})
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("env: %v", err)
	}
	got := strings.Split(strings.TrimSpace(string(out)), "\n")

	for _, want := range []string{
		"HTTPS_PROXY=http://proxy.internal:3128",
		"no_proxy=localhost,127.0.0.1",
		"SSL_CERT_FILE=/etc/obot/ca.pem",
		"SSL_CERT_DIR=/provider/certs",
		"OBOT_AUTH_PROVIDER_EMAIL_DOMAINS=*",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("daemon environment lacks %s: %v", want, got)
		}
	}
	for _, entry := range got {
		if strings.HasPrefix(entry, "OBOT_SECRET_SETTING=") || strings.HasPrefix(entry, "NODE_EXTRA_CA_CERTS=") || entry == "SSL_CERT_DIR=/etc/obot/certs" {
			t.Errorf("daemon environment has unexpected %s", entry)
		}
	}
}
