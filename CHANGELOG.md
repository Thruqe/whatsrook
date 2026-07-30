# Changelog

All notable changes to the WhatsRook project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Added dynamic HTTP port fallback in `client.go`. When the configured HTTP server port (e.g. `3000`) is already bound/in use (`bind: address already in use`), WhatsRook automatically detects the conflict, logs a warning message (`⚠️ Port 3000 was in use — switched to port 3001`), and binds to the next available open port.
- Implemented `.csai` (`.customai`, `.aipersona`, `.aipersonality`) command in `plugins/ai.go` (restricted strictly to Sudoers). Allows owner/sudoers to globally override Meta AI's personality traits and relationship behavior across 10 preset personality traits with paginated interactive buttons + button 11 for custom prompts (e.g. how the AI should refer to the user).
- Added automatic handling for Meta AI server error `488` in `plugins/ai.go`. When Meta AI returns error 488 (uninitialized session), WhatsRook now edits the response to inform the user to start a direct 1-on-1 chat with Meta AI first, and sends a native vCard contact card containing Meta AI's bot JID (`867051314767696@bot`).

- Implemented `.automute`, `.autounmute`, and `.listmute` group management commands (`plugins/automute.go`) with automated background schedule execution.
- Added `.timezone` (`.tz`) configuration command (`plugins/automute.go`) with interactive paginated buttons allowing users to select their local timezone (from all major global timezones) for accurate local schedule execution.
- Ensured `.fetch` command outputs HTTP headers and body responses strictly using standard plain text without custom font transforms (`plugins/fetch.go`).
- Improved YouTube Netscape cookie validation (`plugins/cookie.go`) to strictly require YouTube/Google domains (`youtube.com`, `googlevideo.com`, `google.com`) and reject non-YouTube or social media platform cookies (Facebook, Instagram, Twitter, TikTok, etc.).
- Added WhatsApp Business commands (`plugins/business.go`): `.business` and `.catalog` to fetch business profile info, email, address, operating hours, and business catalog items.
- Added WhatsApp Privacy management command (`plugins/privacy.go`) to view and update account privacy settings (Last Seen, Profile Photo, Status, Read Receipts, Group Add, Online) via interactive buttons.
- Added instability warning notices to outgoing call commands (`.call` and `.videocall`).
- Implemented `.groupcall` command (`plugins/call.go` & `plugins/callplace.go`) using `meowcaller` to initiate group voice/video call sessions.
- Added `status` command (`plugins/statusmenu.go`) allowing owner/sudo users to post status updates (text, image, or video with optional caption) directly to WhatsApp status broadcast (`status.whatsapp.net`).
- Removed unused `meowcaller` directory.
- Removed intermediate `"Running .<command>..."` placeholder status edits so execution status messages are hidden from end users.
- Prevented raw `RUN_COMMAND:` protocol text from being exposed to end users in edited response messages when AI triggers internal commands.
- Updated `autoai` trigger logic to automatically respond when `"Rook"` or `"WhatsRook"` is mentioned in a chat message, in addition to tags and replies.
- Configured system prompt identity so AI adopts the name **WhatsRook** when responding to identity queries.
- Extracted Meta AI system prompt into dedicated prompt file `prompts/meta_ai.txt` with embedded fallback for clean prompt management.
- Added response style guidelines to Meta AI system prompt (prohibiting emojis and mandating a clear, direct, and objective tone).
- Implemented per-chat request queue for Meta AI queries to execute multiple incoming queries sequentially without rejecting concurrent requests.

- Correct Docs url
- Bump deps
- Format docs
- Created dedicated `utils` package for common helper functions across commands.
- Added `utils_test.go` with unit test coverage for package helpers.
- Added automated `CHANGELOG.md` verification workflow (`changelog-check.yml`) in GitHub Actions for per-commit push and pull requests.
- Added Heroku deployment manifests (`app.json` and `Procfile`) and Render configuration (`render.yaml`).
- Implemented outbound video calling support via `!videocall <number>` command (`commands/videocall.go` and `commands/callplace.go`).
- Added automatic connection metadata notification sent directly to the bot owner's DM upon WhatsApp connection (version, git commit hash, session name, OS/Arch, CPU cores, Go runtime).
- Added `IsOwner()` method in `sender/abstract.go` and updated `!delsudo` in `commands/sudo.go` to enforce that only the bot owner can remove users from the sudo list.
- Created Protocol Buffer schema `proto/ws.proto` defining Protobuf message contracts for WebSocket control frames, event frames, and typed payloads (`ControlFrame`, `EventFrame`, `SendMessagePayload`, `IncomingMessagePayload`, etc.).
- Added `scripts/generate-proto.sh` shell script and `make proto` Makefile target to automate Protobuf code generation.
- Added `example/` folder with [`client.go`](./example/client.go) and step-by-step setup documentation in [`README.md`](./example/README.md) demonstrating how to launch the daemon and test Protobuf WebSocket event streaming.

### Changed

- Upgraded `github.com/purpshell/meowcaller` dependency to latest release (`v0.0.0-20260722160050-8e4008f12884`).
- Refactored `commands/helper.go` and command handlers (`call`, `callaudioreply`, `callplace`, `facebook`, `instagram`, `threads`, `tiktok`, `twitter`, `fetch`) to utilize `utils` package functions.
- Updated CLI argument parsing in `cli.go` to support optional boolean values (`--pair=true`, `--qrcode=false`) and environment variable fallbacks (`SESSION`, `PAIR`, `QRCODE`, `CLIENT`, `AUTH_DIR`, `DEBUG`, `VERBOSE`, `DEV`, `LOGOUT`).
- Updated `AGENTS.md` codebase map to document the `utils/` package.
- Reorganized command categories: created new `interactive` category for UI/button/list demonstration commands (`buttons`, `gallery`, `selectlist`, `locbuttons`, `statusmenu`), updated font customization commands to `tools`, normalized `ai` category casing, and unified `filter` commands under `filters`.
- Refactored `main.go` to invoke `runDaemon()` directly in `client.go`, and updated `entrypoint.sh` to accept both `--session <phone>` CLI flags and `$SESSION` environment variables seamlessly.
- Fixed `ParseRunCommand` in `meta/parser.go` to filter out `(link unavailable)` strings and updated `handleAI` streaming in `commands/ai.go` to prevent premature partial `RUN_COMMAND` message edits.
- Updated `ws.go` and `messages.go` to enforce strict Protocol Buffer (`protobuf`) binary transport over WebSockets (`ControlFrame` / `EventFrame`), dropping legacy text JSON handling.
- Updated `example/client.go` to demonstrate pure Protobuf binary event decoding and control requests.

## [4.0.0] - 2026-07-22

### Added

- Shell command execution enhancements and stream handling for AI command invocations.
- Improved media file naming and download processing.
