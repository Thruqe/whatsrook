package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// CLIArgs holds parsed command-line flags.
type CLIArgs struct {
	Session         string
	Pair            bool
	QRCode          bool
	Logout          bool
	Update          bool
	Verbose         bool
	Client          string
	SkipOldMessages bool
}

func parseCLIArgs() CLIArgs {
	loadDotEnv(".env", "../.env")
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
       whatsrook update [check | upgrade]
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

	return CLIArgs{
		Session:         sessionVal,
		Pair:            *pair || getEnvBool("PAIR"),
		QRCode:          *qr || getEnvBool("QRCODE"),
		Logout:          *logout || getEnvBool("LOGOUT"),
		Update:          *update || getEnvBool("UPDATE"),
		Verbose:         *verbose || getEnvBool("VERBOSE"),
		Client:          clientVal,
		SkipOldMessages: !*noSkip,
	}
}

func getEnvBool(key string) bool {
	v := strings.ToLower(os.Getenv(key))
	return v == "true" || v == "1"
}

func loadDotEnv(filenames ...string) {
	for _, filename := range filenames {
		data, err := os.ReadFile(filename)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			if len(val) >= 2 && ((val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'')) {
				val = val[1 : len(val)-1]
			}
			if os.Getenv(key) == "" {
				_ = os.Setenv(key, val)
			}
		}
	}
}
