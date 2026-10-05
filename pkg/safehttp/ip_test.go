package safehttp

import (
	"net"
	"testing"
)

func TestIPClassification(t *testing.T) {
	tests := []struct {
		name      string
		ip        string
		loopback  bool
		private   bool
		linkLocal bool
	}{
		{
			name: "public IPv4",
			ip:   "8.8.8.8",
		},
		{
			name: "public IPv6",
			ip:   "2606:4700:4700::1111",
		},
		{
			name:     "unspecified IPv4",
			ip:       "0.0.0.0",
			loopback: true,
		},
		{
			name:     "unspecified IPv6",
			ip:       "::",
			loopback: true,
		},
		{
			name:      "NAT64 metadata",
			ip:        "64:ff9b::a9fe:a9fe",
			linkLocal: true,
		},
		{
			name:     "NAT64 local-use loopback",
			ip:       "64:ff9b:1::7f00:1",
			loopback: true,
		},
		{
			name:    "6to4 private",
			ip:      "2002:0a00:0001::",
			private: true,
		},
		{
			name:      "6to4 metadata",
			ip:        "2002:a9fe:a9fe::",
			linkLocal: true,
		},
		{
			name:     "IPv4-compatible loopback",
			ip:       "::7f00:1",
			loopback: true,
		},
		{
			name:      "IPv4-compatible metadata",
			ip:        "::a9fe:a9fe",
			linkLocal: true,
		},
		{
			name:    "Teredo obfuscated private client",
			ip:      "2001:0:4136:e378:8000:63bf:f5ff:fffe",
			private: true,
		},
		{
			name: "NAT64 public",
			ip:   "64:ff9b::808:808",
		},
		{
			name: "6to4 public",
			ip:   "2002:808:808::",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("failed to parse %s", tt.ip)
			}
			if got := IsLoopback(ip); got != tt.loopback {
				t.Errorf("IsLoopback(%s) = %v, want %v", tt.ip, got, tt.loopback)
			}
			if got := IsPrivate(ip); got != tt.private {
				t.Errorf("IsPrivate(%s) = %v, want %v", tt.ip, got, tt.private)
			}
			if got := IsLinkLocal(ip); got != tt.linkLocal {
				t.Errorf("IsLinkLocal(%s) = %v, want %v", tt.ip, got, tt.linkLocal)
			}
		})
	}
}

func TestClientBlocksUnspecifiedIP(t *testing.T) {
	_, err := NewClient(Options{BlockLoopback: true}).Get("http://0.0.0.0:8080/")
	if err == nil {
		t.Fatal("expected unspecified IP to be blocked")
	}
}

func TestClientBlocksNAT64EmbeddedLinkLocalIP(t *testing.T) {
	_, err := NewClient(Options{BlockLinkLocal: true}).Get("http://[64:ff9b::a9fe:a9fe]/latest/meta-data")
	if err == nil {
		t.Fatal("expected NAT64 address embedding a link-local IP to be blocked")
	}
}
