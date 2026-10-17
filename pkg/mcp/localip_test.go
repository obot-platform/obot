package mcp

import (
	"errors"
	"net"
	"testing"
)

func TestFirstInterfaceIP(t *testing.T) {
	cidr := func(s string) net.Addr {
		ip, n, err := net.ParseCIDR(s)
		if err != nil {
			t.Fatal(err)
		}
		n.IP = ip
		return n
	}
	tests := []struct {
		name  string
		addrs []net.Addr
		err   error
		want  string
	}{
		{
			name:  "skips loopback and link-local, prefers IPv4",
			addrs: []net.Addr{cidr("127.0.0.1/8"), cidr("::1/128"), cidr("fe80::1/64"), cidr("fd00::5/64"), cidr("172.18.0.6/16")},
			want:  "172.18.0.6",
		},
		{
			name:  "falls back to IPv6",
			addrs: []net.Addr{cidr("127.0.0.1/8"), cidr("fd00::5/64")},
			want:  "fd00::5",
		},
		{
			name:  "nothing usable",
			addrs: []net.Addr{cidr("127.0.0.1/8")},
		},
		{
			name: "interfaces can't be listed",
			err:  errors.New("boom"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstInterfaceIP(func() ([]net.Addr, error) { return tt.addrs, tt.err })
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
