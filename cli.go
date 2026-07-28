// Command-line flag parsing and configuration types.
package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

// ClientType represents the platform emulated by the WhatsApp client.
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

// CliArgs holds all parsed command-line arguments and environment overrides.
type CliArgs struct {
	Session         string
	Pair            bool
	Port            string
	AuthDir         string
	QRCode          bool
	Logout          bool
	Update          bool
	Debug           bool
	Verbose         bool
	Dev             bool
	Client          ClientType
	SkipOldMessages bool
}

func parseArgs() CliArgs {
	args := os.Args[1:]

	for _, a := range args {
		if a == "-h" || a == "--help" {
			fmt.Print(`Usage: whatsrook -session <phone_number> [OPTIONS]
       whatsrook --update

Options:
  -s, --session <phone>  Phone number used to identify the session (required unless --update)
  -p, --pair             Request a pair code using the --session phone number
  -P, --port <port>      Specify the HTTP/WebSocket port (default: 3000, or $PORT)
  -a, --auth-dir <path>  Directory to store session auth files (default: ./auth)
  -c, --client <type>    Client type: chrome (default), android, ios
  -q, --qrcode           Print the QR code to stdout for scanning
  -l, --logout           Remove the session auth files and exit
  -u, --update           Check and perform application update, then exit or restart
  -v, --verbose          Enable verbose logging for application (excluding whatsmeow)
  -d, --dev              Dev mode: disables CORS origin check on WebSocket
  --no-skip-old          Process messages sent while the bot was offline (default: skip them)
  -h, --help             Show this help message
`)
			os.Exit(0)
		}
	}

	getValue := func(longFlag, shortFlag string) string {
		for i, a := range args {
			if (a == longFlag || a == shortFlag) && i+1 < len(args) {
				return args[i+1]
			}
		}
		return ""
	}

	getBoolFlag := func(longFlag, shortFlag, envVar string) bool {
		for i, a := range args {
			if a == longFlag || a == shortFlag {
				if i+1 < len(args) && (args[i+1] == "true" || args[i+1] == "false") {
					return args[i+1] == "true"
				}
				return true
			}
			if after, ok := strings.CutPrefix(a, longFlag+"="); ok {
				val := after
				return val == "true" || val == "1"
			}
			if shortFlag != "" && strings.HasPrefix(a, shortFlag+"=") {
				val := strings.TrimPrefix(a, shortFlag+"=")
				return val == "true" || val == "1"
			}
		}
		if envVar != "" {
			envVal := strings.ToLower(os.Getenv(envVar))
			return envVal == "true" || envVal == "1"
		}
		return false
	}

	isUpdate := getBoolFlag("--update", "-u", "UPDATE")

	session := getValue("--session", "-s")
	if session == "" {
		session = os.Getenv("SESSION")
	}
	if session == "" && !isUpdate {
		fmt.Fprintln(os.Stderr, "Error: --session <phone_number> or $SESSION environment variable is required. Run with -h for help.")
		os.Exit(1)
	}

	client := ClientChrome
	if raw := getValue("--client", "-c"); raw != "" {
		c, ok := parseClientType(raw)
		if !ok {
			fmt.Fprintf(os.Stderr, "Error: unknown --client %q. Valid options: chrome, android, ios\n", raw)
			os.Exit(1)
		}
		client = c
	} else if rawEnv := os.Getenv("CLIENT"); rawEnv != "" {
		if c, ok := parseClientType(rawEnv); ok {
			client = c
		}
	}

	port := getValue("--port", "-P")
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "3000"
	}

	authDir := getValue("--auth-dir", "-a")
	if authDir == "" {
		authDir = os.Getenv("AUTH_DIR")
	}
	if authDir == "" {
		authDir = "auth"
	}

	skipOld := !slices.Contains(args, "--no-skip-old")

	return CliArgs{
		Session:         session,
		Pair:            getBoolFlag("--pair", "-p", "PAIR"),
		Port:            port,
		AuthDir:         authDir,
		QRCode:          getBoolFlag("--qrcode", "-q", "QRCODE"),
		Logout:          getBoolFlag("--logout", "-l", "LOGOUT"),
		Update:          isUpdate,
		Verbose:         getBoolFlag("--verbose", "-v", "VERBOSE"),
		Dev:             getBoolFlag("--dev", "-d", "DEV"),
		Client:          client,
		SkipOldMessages: skipOld,
	}
}
