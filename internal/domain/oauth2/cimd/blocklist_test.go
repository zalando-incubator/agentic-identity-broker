package cimd

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSRFBlocklist(t *testing.T) {
	bl, err := NewSSRFBlocklist(nil)
	require.NoError(t, err)

	blocked := []struct {
		name string
		ip   string
	}{
		{"loopback IPv4", "127.0.0.1"},
		{"loopback IPv4 other", "127.0.0.100"},
		{"loopback IPv6", "::1"},
		{"RFC1918 10.x", "10.0.0.1"},
		{"RFC1918 172.16.x", "172.16.0.1"},
		{"RFC1918 172.31.x", "172.31.255.255"},
		{"RFC1918 192.168.x", "192.168.1.1"},
		{"link-local IPv4", "169.254.169.254"},
		{"link-local IPv4 other", "169.254.0.1"},
		{"link-local IPv6", "fe80::1"},
		{"this-network", "0.0.0.1"},
		{"CGN", "100.64.0.1"},
		{"documentation TEST-NET-1", "192.0.2.1"},
		{"documentation TEST-NET-2", "198.51.100.1"},
		{"documentation TEST-NET-3", "203.0.113.1"},
		{"benchmarking", "198.18.0.1"},
		{"6to4 relay anycast", "192.88.99.1"},
		{"reserved", "240.0.0.1"},
		{"broadcast", "255.255.255.255"},
		{"unique local IPv6", "fc00::1"},
		{"documentation IPv6", "2001:db8::1"},
		{"NAT64 well-known prefix", "64:ff9b::a9fe:a9fe"},
		{"NAT64 local-use prefix", "64:ff9b:1::a9fe:a9fe"},
		{"6to4 embeds private IPv4", "2002:0a00:0001::"},
		{"Teredo embeds private IPv4", "2001:0:0a00:0001::"},
		{"multicast IPv4", "224.0.0.1"},
		{"multicast IPv6", "ff02::1"},
		{"unspecified IPv6", "::"},
	}

	for _, tc := range blocked {
		t.Run("blocks "+tc.name, func(t *testing.T) {
			assert.True(t, bl.Contains(net.ParseIP(tc.ip)), "expected %s to be blocked", tc.ip)
		})
	}

	allowed := []struct {
		name string
		ip   string
	}{
		{"public IPv4", "8.8.8.8"},
		{"public IPv4 other", "93.184.216.34"},
		{"public IPv6", "2001:4860:4860::8888"},
	}

	for _, tc := range allowed {
		t.Run("allows "+tc.name, func(t *testing.T) {
			assert.False(t, bl.Contains(net.ParseIP(tc.ip)), "expected %s to be allowed", tc.ip)
		})
	}

	t.Run("accepts extra operator CIDRs", func(t *testing.T) {
		bl2, err := NewSSRFBlocklist([]string{"203.0.114.0/24"})
		require.NoError(t, err)
		assert.True(t, bl2.Contains(net.ParseIP("203.0.114.1")))
		assert.False(t, bl2.Contains(net.ParseIP("203.0.115.1")))
	})

	t.Run("rejects malformed extra CIDR", func(t *testing.T) {
		_, err := NewSSRFBlocklist([]string{"not-a-cidr"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid blocked CIDR")
	})
}
