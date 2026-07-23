// Configuration types for the example WebSocket client.
package main

// Config defines execution flags and behavior for the example client.
type Config struct {
	// Enable QR code request / log output handling
	QRCODE bool

	// Enable phone pair code request flow
	PAIR bool

	// Trigger automated logout after 30 seconds of active connection
	LOGOUT bool
}

// DefaultConfig provides default configuration options for the example client.
var DefaultConfig = Config{
	QRCODE: false,
	PAIR:   false,
	LOGOUT: false,
}
