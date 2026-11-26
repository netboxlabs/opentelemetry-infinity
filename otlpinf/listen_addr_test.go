package otlpinf

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An IPv6 literal carries the colons the port separator uses, so the address
// has to bracket the host; unbracketed, net.Listen refuses it.
func TestListenAddrBracketsIPv6(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{"127.0.0.1", "127.0.0.1:10222"},
		{"localhost", "localhost:10222"},
		{"", ":10222"},
		{"::1", "[::1]:10222"},
		{"::", "[::]:10222"},
		{"2001:db8::1", "[2001:db8::1]:10222"},
		{"fe80::1%eth0", "[fe80::1%eth0]:10222"},
		{"[::1]", "[::1]:10222"},
		{"[fe80::1%eth0]", "[fe80::1%eth0]:10222"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, listenAddr(tt.host, 10222), "host %q", tt.host)
	}
}

// The address has to be one the listener accepts. A loopback the platform
// cannot bind is skipped, established with an address known to parse.
func TestListenAddrIsBindable(t *testing.T) {
	for host, bracketed := range map[string]string{"127.0.0.1": "127.0.0.1:0", "::1": "[::1]:0", "[::1]": "[::1]:0"} {
		probe, err := net.Listen("tcp", bracketed)
		if err != nil {
			t.Logf("skipping %s, the platform will not bind it: %v", host, err)
			continue
		}
		require.NoError(t, probe.Close())

		ln, err := net.Listen("tcp", listenAddr(host, 0))
		require.NoError(t, err, "host %q rendered as %q", host, listenAddr(host, 0))
		require.NoError(t, ln.Close())
	}
}
