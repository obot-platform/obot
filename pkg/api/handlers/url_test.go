package handlers

import "testing"

func TestNormalizeOrigin(t *testing.T) {
	t.Parallel()

	origin, hostname, err := NormalizeOrigin(" http://Obot.default.svc.cluster.local/ ")
	if err != nil {
		t.Fatal(err)
	}
	if origin != "http://Obot.default.svc.cluster.local" {
		t.Fatalf("expected normalized origin, got %q", origin)
	}
	if hostname != "obot.default.svc.cluster.local" {
		t.Fatalf("expected lowercase hostname, got %q", hostname)
	}

	if _, _, err := NormalizeOrigin("http://obot.default.svc.cluster.local/path"); err == nil {
		t.Fatal("expected path to be rejected")
	}
	if _, _, err := NormalizeOrigin("obot.default.svc.cluster.local"); err == nil {
		t.Fatal("expected missing scheme to be rejected")
	}
}

func TestHostnameMatches(t *testing.T) {
	t.Parallel()

	for _, host := range []string{
		"Obot.default.svc.cluster.local",
		"obot.default.svc.cluster.local:80",
	} {
		if !HostnameMatches(host, "obot.default.svc.cluster.local") {
			t.Fatalf("expected %q to match", host)
		}
	}

	if !HostnameMatches("[fd00::1]:80", "fd00::1") {
		t.Fatal("expected bracketed IPv6 host with port to match")
	}
	if HostnameMatches("external.example.com", "obot.default.svc.cluster.local") {
		t.Fatal("expected different hostnames not to match")
	}
}
