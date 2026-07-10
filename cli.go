package main

import (
	"fmt"
	"os"
	"strings"
)

type ClientType int

const (
	ClientChrome ClientType = iota
	ClientAndroid
	ClientIos
)

func parseClientType(s string) (ClientType, bool) {
	switch strings.ToLower(s) {
	case "chrome":
		return ClientChrome, true
	case "android":
		return ClientAndroid, true
	case "ios":
		return ClientIos, true
	default:
		return ClientChrome, false
	}
}

type CliArgs struct {
	Session string
	Pair    string // empty = not set
	Port    string
	AuthDir string
	QRCode  bool
	Logout  bool
	Debug   bool
	Client  ClientType
}

func parseArgs() CliArgs {
	args := os.Args[1:]

	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Print(`Usage: zevBot --session <phone_number> [OPTIONS]

Options:
  --session <phone>     Phone number used to identify the session (required)
  --pair <phone>        Request a pair code for the given phone number
  --port <port>         Specify the HTTP/WebSocket port
  --auth-dir <path>     Directory to store session auth files (default: ./auth)
  --client <type>       Client type: chrome (default), android, ios
  --qrcode              Print the QR code to stdout for scanning
  --logout              Remove the session auth files and exit
  --debug               Enable debug logging
  -h, --help            Show this help message
`)
			os.Exit(0)
		}
	}

	getValue := func(flag string) string {
		for i, a := range args {
			if a == flag && i+1 < len(args) {
				return args[i+1]
			}
		}
		return ""
	}

	hasFlag := func(flag string) bool {
		for _, a := range args {
			if a == flag {
				return true
			}
		}
		return false
	}

	session := getValue("--session")
	if session == "" {
		fmt.Fprintln(os.Stderr, "Error: --session <phone_number> is required. Run with -h for help.")
		os.Exit(1)
	}

	client := ClientChrome
	if raw := getValue("--client"); raw != "" {
		c, ok := parseClientType(raw)
		if !ok {
			fmt.Fprintf(os.Stderr, "Error: unknown --client %q. Valid options: chrome, android, ios\n", raw)
			os.Exit(1)
		}
		client = c
	}

	port := getValue("--port")
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "3000"
	}

	authDir := getValue("--auth-dir")
	if authDir == "" {
		authDir = "auth"
	}

	return CliArgs{
		Session: session,
		Pair:    getValue("--pair"),
		Port:    port,
		AuthDir: authDir,
		QRCode:  hasFlag("--qrcode"),
		Logout:  hasFlag("--logout"),
		Debug:   hasFlag("--debug"),
		Client:  client,
	}
}
