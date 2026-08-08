package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"whatsrook"
	"whatsrook/cli/updater"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "update" {
		ctx := context.Background()
		up := updater.New(updater.Options{
			Out: os.Stdout,
		})

		subcmd := "check"
		if len(os.Args) > 2 {
			subcmd = os.Args[2]
		}

		switch subcmd {
		case "check":
			_, err := up.Check(ctx)
			if err != nil {
				slog.Error("update check failed", "err", err)
				os.Exit(1)
			}
			return

		case "upgrade", "apply", "now":
			res, err := up.Upgrade(ctx, false)
			if err != nil {
				slog.Error("upgrade failed", "err", err)
				os.Exit(1)
			}
			if res.Updated {
				fmt.Println("==> Restarting process...")
				if err := updater.RestartProcess(); err != nil {
					slog.Error("failed to restart process", "err", err)
					os.Exit(1)
				}
			}
			return

		default:
			fmt.Fprintf(os.Stderr, "Unknown update subcommand %q. Usage: whatsrook update [check|upgrade]\n", subcmd)
			os.Exit(1)
		}
	}

	args := parseCLIArgs()

	if args.Update {
		ctx := context.Background()
		up := updater.New(updater.Options{
			Out: os.Stdout,
		})

		res, err := up.Upgrade(ctx, false)
		if err != nil {
			slog.Error("update failed", "err", err)
			os.Exit(1)
		}

		if res.Updated {
			fmt.Println("==> Restarting process...")
			if err := updater.RestartProcess(); err != nil {
				slog.Error("failed to restart process", "err", err)
				os.Exit(1)
			}
		}
		return
	}

	clientType, ok := whatsrook.ParseClientType(args.Client)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: unknown --client %q. Valid options: chrome, android, ios\n", args.Client)
		os.Exit(1)
	}

	if args.Session == "" {
		fmt.Fprintln(os.Stderr, "Error: --session <phone_number> or $SESSION environment variable is required. Run with -h for help.")
		os.Exit(1)
	}

	rook := whatsrook.NewRookClient(whatsrook.Config{
		Session:         args.Session,
		Pair:            args.Pair,
		QRCode:          args.QRCode,
		Logout:          args.Logout,
		Verbose:         args.Verbose,
		ClientType:      clientType,
		SkipOldMessages: args.SkipOldMessages,
	})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := rook.Start(ctx); err != nil {
		slog.Error("rook client error", "err", err)
		os.Exit(1)
	}
}
