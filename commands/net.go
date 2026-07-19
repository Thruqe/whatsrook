package commands

import (
	"context"
	"net"
)

func init() {
	// Force the standard library dialer to prefer IPv4 networks globally
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			var d net.Dialer
			// Upgrade any dual-stack network target strings directly to IPv4
			if network == "tcp" || network == "tcp6" {
				network = "tcp4"
			}
			if network == "udp" || network == "udp6" {
				network = "udp4"
			}
			return d.DialContext(ctx, network, address)
		},
	}
}
