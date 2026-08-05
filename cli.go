package whatsrook

import (
	"flag"
	"fmt"
	"os"
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
	c, ok := map[string]ClientType{"chrome": ClientChrome, "android": ClientAndroid, "ios": ClientIos}[strings.ToLower(s)]
	return c, ok
}

// Arguments holds all parsed command-line arguments and environment overrides.
type Arguments struct {
	Session         string
	Pair            bool
	QRCode          bool
	Logout          bool
	Update          bool
	Verbose         bool
	Client          ClientType
	SkipOldMessages bool
}

func parseArgs() Arguments {
	fs := flag.NewFlagSet("whatsrook", flag.ExitOnError)

	var (
		session = fs.String("s", os.Getenv("SESSION"), "")
		pair    = fs.Bool("p", false, "")
		client  = fs.String("c", "chrome", "")
		qr      = fs.Bool("q", false, "")
		logout  = fs.Bool("l", false, "")
		update  = fs.Bool("u", false, "")
		verbose = fs.Bool("v", false, "")
		noSkip  = fs.Bool("no-skip-old", false, "")
	)

	fs.StringVar(session, "session", *session, "")
	fs.BoolVar(pair, "pair", *pair, "")
	fs.StringVar(client, "client", *client, "")
	fs.BoolVar(qr, "qrcode", *qr, "")
	fs.BoolVar(logout, "logout", *logout, "")
	fs.BoolVar(update, "update", *update, "")
	fs.BoolVar(verbose, "verbose", *verbose, "")

	fs.Usage = func() {
		fmt.Print(`Usage: whatsrook -session <phone_number> [OPTIONS]
       whatsrook --update

Options:
  -s, --session <phone>  Phone number used to identify the session (required unless --update)
  -p, --pair             Request a pair code using the --session phone number
  -c, --client <type>    Client type: chrome (default), android, ios
  -q, --qrcode           Print the QR code to stdout for scanning
  -l, --logout           Remove the session auth files and exit
  -u, --update           Check and perform application update, then exit or restart
  -v, --verbose          Enable verbose logging
  --no-skip-old          Process messages sent while the bot was offline (default: skip them)
  -h, --help             Show this help message
`)
	}

	_ = fs.Parse(os.Args[1:])

	sessionVal := *session
	if sessionVal == "" && fs.NArg() > 0 {
		for _, arg := range fs.Args() {
			cleanArg := strings.TrimPrefix(arg, "+")
			if len(cleanArg) >= 7 && len(cleanArg) <= 15 {
				allDigits := true
				for _, r := range cleanArg {
					if r < '0' || r > '9' {
						allDigits = false
						break
					}
				}
				if allDigits {
					sessionVal = arg
					break
				}
			}
		}
	}

	clientVal := *client
	if clientVal == "chrome" && os.Getenv("CLIENT") != "" {
		clientVal = os.Getenv("CLIENT")
	}

	c, ok := parseClientType(clientVal)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: unknown --client %q. Valid options: chrome, android, ios\n", clientVal)
		os.Exit(1)
	}

	if sessionVal == "" && !*update {
		fmt.Fprintln(os.Stderr, "Error: --session <phone_number> or $SESSION environment variable is required. Run with -h for help.")
		os.Exit(1)
	}

	return Arguments{
		Session:         sessionVal,
		Pair:            *pair || getEnvBool("PAIR"),
		QRCode:          *qr || getEnvBool("QRCODE"),
		Logout:          *logout || getEnvBool("LOGOUT"),
		Update:          *update || getEnvBool("UPDATE"),
		Verbose:         *verbose || getEnvBool("VERBOSE"),
		Client:          c,
		SkipOldMessages: !*noSkip,
	}
}

func getEnvBool(key string) bool {
	v := strings.ToLower(os.Getenv(key))
	return v == "true" || v == "1"
}
