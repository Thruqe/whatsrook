# Changelog

All notable changes to the WhatsRook project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Created dedicated `utils` package for common helper functions across commands.
- Added `utils_test.go` with unit test coverage for package helpers.
- Added automated `CHANGELOG.md` verification workflow (`changelog-check.yml`) in GitHub Actions.
- Added Heroku deployment manifests (`app.json` and `Procfile`) and Render configuration (`render.yaml`).
- Implemented outbound video calling support via `!videocall <number>` command (`commands/videocall.go` and `commands/callplace.go`).
- Added automatic connection metadata notification sent directly to the bot owner's DM upon WhatsApp connection (version, git commit hash, session name, OS/Arch, CPU cores, Go runtime).
- Added `IsOwner()` method in `sender/abstract.go` and updated `!delsudo` in `commands/sudo.go` to enforce that only the bot owner can remove users from the sudo list.

### Changed
- Upgraded `github.com/purpshell/meowcaller` dependency to latest release (`v0.0.0-20260722160050-8e4008f12884`).
- Refactored `commands/helper.go` and command handlers (`call`, `callaudioreply`, `callplace`, `facebook`, `instagram`, `threads`, `tiktok`, `twitter`, `fetch`) to utilize `utils` package functions.
- Updated CLI argument parsing in `cli.go` to support optional boolean values (`--pair=true`, `--qrcode=false`) and environment variable fallbacks (`SESSION`, `PAIR`, `QRCODE`, `CLIENT`, `AUTH_DIR`, `DEBUG`, `VERBOSE`, `DEV`, `LOGOUT`).
- Updated `AGENTS.md` codebase map to document the `utils/` package.
- Reorganized command categories: created new `interactive` category for UI/button/list demonstration commands (`buttons`, `gallery`, `selectlist`, `locbuttons`, `statusmenu`), updated font customization commands to `tools`, normalized `ai` category casing, and unified `filter` commands under `filters`.

## [4.0.0] - 2026-07-22

### Added
- Shell command execution enhancements and stream handling for AI command invocations.
- Improved media file naming and download processing.
