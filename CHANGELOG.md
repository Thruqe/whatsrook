# Changelog

All notable changes to the WhatsRook project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Group & Community Commands (`plugins/group.go`):
  - `.kickall`: Group-only command that removes all group participants except the bot itself, the invoker, and sudoers (requires bot admin status).
  - `.community` (Aliases: `listgroups`, `groupslist`, `allgroups`): Lists all joined/community groups along with their active invite URLs.
  - `.leave` (Alias: `left`): Group-only command displaying interactive "Confirm Leave" and "Cancel" buttons. Button confirmation/cancellation is strictly locked to the command invoker's User ID (other users tapping the button are politely rejected).
- Word Prefix & Natural Language Command Routing (`plugins/dispatch.go`, `plugins/sh.go`):
  - Removed `run` from `sh` (shell execution) command aliases to prevent natural language phrases like `rook, run menu command` from attempting to execute system binaries.
  - Enhanced command parsing to trim leading punctuation (commas, colons) following word prefixes (`rook, ...`).
  - Added smart command extraction in `runCommand`: if a natural language phrasing is used (e.g. `rook, run menu command`), it identifies registered bot commands (`menu`) within the text and executes them seamlessly.
- Cookie Editor Extension Download Link (`plugins/cookie.go`, `plugins/play.go`):
  - Integrated `https://cookie-editor.com/#download` extension URL into `.cookie` tutorial instructions, `.setcookie` usage hints, validation errors, and `.play` download cookie failure prompts.
- Release Package Asset Packaging & Updater Support (`.github/workflows/release.yml`, `Dockerfile`, `updater/updater.go`):
  - Updated GitHub Actions release packaging ([.github/workflows/release.yml](file:///home/thruqe/whatsrook/.github/workflows/release.yml)) to include both `resources/` and `prompts/` directories in the output `.tar.gz` release archives.
  - Updated `Dockerfile` to copy `resources/` and `prompts/` into release Docker container images.
  - Updated `updater/updater.go` to unpack `resources/` and `prompts/` directory trees from release tarballs during automatic binary updates.
- Leaderboard User JID Normalization & Deduplication (`plugins/helper.go`, `plugins/tictactoe.go`, `plugins/wcg.go`, `plugins/unscramble.go`):
  - Created `NormalizeUserJID` to consistently map LID (`@lid`) and Phone JID (`@s.whatsapp.net`) entries to a single primary user identity.
  - Added in-memory entry merging in group leaderboards (`.leaderboard`, `.lb`) to combine XP, stats, and rankings so no player appears duplicated across LID and Phone JID rows.
- Word Prefix & AutoAI Command Fallthrough Routing (`plugins/dispatch.go`):
  - Fixed command dispatch when a word prefix (e.g. `rook` / `whatsrook`) matches: if the text after the prefix is a valid command (e.g. `rook ping`), it executes the command handler. If it is not a command (e.g. `rook hey bro how are you`), it now correctly falls through to AutoAI when enabled.
- Interactive Button Mentions & Raw JID Preservation (`sender/abstract.go`, `plugins/movie.go`):
  - Updated `sendInteractiveButtonsWithMentions` to populate `ButtonsMessage.ContextInfo.MentionedJID` when sending interactive button cards containing user mentions.
  - Preserved raw JID/LID format in `ResolveMentionRaw` and `MentionedJID` arrays so LID/JID references are correctly parsed and rendered by WhatsApp clients.
- Native `@all` Group Mention for `.tagall` (`plugins/group.go`, `sender/abstract.go`):
  - Updated `.tagall` command to use WhatsApp's exact native `@all` group mention stanza using `NonJIDMentions: 1` in `ContextInfo` matching the official WhatsApp web/app payload.
- AutoBio Command & Scheduler (`plugins/autobio.go`, `plugins/dispatch.go`):
  - Created `.autobio` command (`.autobio on`, `.autobio off`, `.autobio toggle`, `.autobio tz <TZ>`, `.autobio status`, `.autobio now`).
  - Added background 1-minute ticker scheduler (`StartAutoBioScheduler`) that automatically updates the WhatsApp status bio (`client.SetStatusMessage`) with local time and inspirational quotes.
  - Supports configurable timezones (e.g. `Africa/Lagos`, `America/New_York`, `UTC`, `Europe/London`).
  - Includes interactive setting menu buttons for status toggling and manual status bio updates.
- Word Chain Game (`.wcg`) End Command & Button (`plugins/wcg.go`):
  - Added `.wcg end` subcommand (alongside `.wcg cancel` / `.wcg stop`) allowing any player to terminate an active WCG match or lobby window.
  - Added interactive `"End Game"` button (`.wcg end`) to WCG lobby menus and in-game turn notifications so players can end the game directly with a single tap.
- Updated `.savecontact` DM Auto-Targeting & PushName Detection (`plugins/savecontact.go`):
  - In direct messages (DM), `.savecontact` automatically targets the DM peer (`ctx.Chat`).
  - If an explicit name is not provided as an argument, `.savecontact` attempts to auto-detect the user's PushName from event info or local contact store cache.
  - If no PushName can be auto-detected and no name argument is provided, prompts the user to specify a name.
- Updated `.savecontact` JID/LID AppState mutation handling (`plugins/savecontact.go`):
  - Fixed mutation index key to `appstate.IndexContact` ("contact") so WhatsApp servers recognize contact sync actions.
  - Fixed `PutContactName` argument order (`firstName, fullName`) when caching contact names in the local database store.
  - Automatically maps both `PnJID` and `LidJID` in `waSyncAction.ContactAction` when available.
  - Converted internal logging to `log/slog` (`slog.Debug`, `slog.Error`).
- Call Command Debug Logging (`plugins/callplace.go`, `plugins/callaudioreply.go`):
  - Replaced stdout `log.Printf` calls across call handlers with `slog.Debug` and `slog.Error` from `"log/slog"`.
  - Call operation logs are now strictly DEBUG level and only output when the verbose (`-v` / `--verbose`) flag is enabled.
- Configurable Bot Display Name (`plugins/botname.go`, `sender/context.go`, `meta/parser.go`, `plugins/dispatch.go`):
  - Added `.botname` / `.setbotname` command allowing users to view and customize their bot's display name (e.g., `.botname Jarvis` or `.botname reset`).
  - Integrated `ctx.GetBotName()` across `.menu`, AI response system prompts, sticker metadata, buttons/footers, and connection banners.
  - Meta AI system instructions dynamically adopt the custom bot name (e.g., "Your name is Jarvis...").
  - AutoAI trigger keywords automatically match the configured bot name in addition to "WhatsRook" / "Rook".
- Updated Protobuf pointer constructors to `new(...)` (`plugins/savecontact.go`, `sender/abstract.go`): replaced legacy `proto.Bool` / `proto.String` helper constructors with Go's `new(...)` pointer initializer.
- Rebuilt `.savecontact` with Protobuf AppState SyncAction (`plugins/savecontact.go`):
  - Removed outdated privacy check heuristics.
  - Implemented WhatsApp AppState patch sync (`appstate.WAPatchCriticalUnblockLow`, `Version: 2`, `Index: [appstate.IndexContact, jid]`) containing `waSyncAction.SyncActionValue{ ContactAction: ... }`.
  - Dispatches contact synchronization via `ctx.Client.SendAppState(ctx.Ctx, patch)` to persist contact names across linked devices and servers.
- Word Prefix Support (`plugins/prefix.go`, `plugins/dispatch.go`, `sender/context.go`):
  - Fixed `.prefix` command so word prefixes (e.g. `jarvis`, `bot`, `rook`) are preserved as full words rather than being split into single-character symbols.
  - Added case-insensitive word-boundary prefix matching in `Dispatch` (`plugins/dispatch.go`). Commands can now be triggered with word prefixes like `jarvis ping`, `Jarvis ping`, `JARVIS menu`, or `jarvis`.
  - Updated `Context.GetPrefix()` to automatically format usage strings cleanly for word prefixes (`jarvis ping` vs `.ping`).
- Fixed `meowcaller` group calling integration (`plugins/call.go` & `plugins/callplace.go`):
  - Updated `placeGroupCall` to pass `meowcaller.GroupCallOptions{GroupJID: groupJID}`, properly binding group calls to WhatsApp groups using `GroupCallByIDWithOptions` (when calling all remote group members) and `GroupCallWithOptions` (when calling target participants).
  - All group call initiation and termination status messages now output clean `<Group Name>` text instead of raw group JID strings.
- Formatted User Mentions & Group Names in Call Commands (`plugins/callplace.go`):
  - `.call`, `.videocall`, and `.groupcall` response messages no longer output raw JID strings (`258256953950323@lid` or group JIDs) to end users.
  - Target call participants are now properly formatted as `@user` tags with WhatsApp metadata mentions via `ReplyWithMentions`.
  - Group calls now dynamically display the Group Name instead of raw group JIDs.
- Preserved raw JID and LID formatting in mention arrays (`sender/abstract.go`, `events.go`, `plugins/dispatch.go`): `ResolveMentionRaw` and mention builders now preserve the exact raw JID/LID strings (including LID server mappings and device AD suffixes) without calling `.ToNonAD()`. This ensures mentions in call commands, group greetings, and system messages are correctly parsed by WhatsApp clients.
- Per-Group Leaderboard System (`bot_group_user_xp` table):
  - `.leaderboard` (`.lb`, `.top`, `.xp`) is now group-specific. When run in a group chat, it dynamically resolves the group's name and displays `<Group Name> Leaderboard`.
  - Added DM score exclusion: Games played in Direct Messages (P2P chats) will no longer record XP or game statistics to any group leaderboard.
  - Updated all game stat save handlers (`.wcg`, `.unscramble`, `.ttt`) to store stats isolated per group JID.
- Fixed reentrant mutex deadlock in `.wcg` and `.unscramble` game turn handlers (`plugins/wcg.go` & `plugins/unscramble.go`): `eliminateAndAdvanceWCG` and turn timer callbacks no longer hold `game.Mu` before invoking `game.EliminateCurrentPlayer()`, `game.StopTimers()`, or `finishWCGChainGame()`. This resolves the deadlock that prevented the match-over leaderboard from being sent and caused the dispatch loop to freeze.
- Fixed command dispatch locking issue during active `.wcg` and `.unscramble` games (`plugins/wcg.go` & `plugins/unscramble.go`): `HandleWCGInput` and `HandleUnscrambleInput` now return `false` if the message sender is not in the game or is not the current turn player. This ensures messages from non-turn players or new command invocations (like `.wcg`, `.menu`, `.ping`) pass through to `Dispatch` uninterrupted.
- Fixed match-over leaderboard display in `plugins/wcg.go`: `finishWCGChainGame` now always outputs the full `Final Standings:` leaderboard table upon match conclusion (for single-player eliminations, all-player eliminations, and multiplayer last-standing finishes alike).
- Updated `utils/wcg.go` so `EliminateCurrentPlayer` returns `gameOver = true` when 1 active player remains (as well as 0). This ensures the game immediately ends and displays the complete final leaderboard standings and winner/highest-scorer evaluation once all other players are eliminated.
- Preserved raw JID/LIDs without stripping or altering them in `SendTextWithMentions` and `ReplyWithMentions` in `sender/abstract.go` so WhatsApp client mention popups work seamlessly.
- Updated `.wcg` (**Word Chain Game**):
  - Dynamic Turn Time Limits: Time limit now decreases automatically round by round starting at 25 seconds down to a minimum of 6 seconds (`25s - (round-1)*2s`).
  - Immediate Player Elimination & Match End: When a player submits an invalid word (too short, wrong starting letter, duplicate, or unrecognized word), they are immediately eliminated from the match and their active turn timer is cancelled. If 0 active players remain, the match immediately ends and prints the final leaderboard standings.
- Created new `.wcg` (**Word Chain Game**) command (`plugins/wcg.go` & `utils/wcg.go`). Players submit valid English words starting with a random required letter that meet or exceed the required character length. Word validity is verified in parallel across 5 English dictionary APIs (`api.dictionaryapi.dev`, `api.datamuse.com`, Wiktionary API, US/UK dictionary proxies) plus built-in fallback dictionaries.
- Fixed `meowcaller` client binding across packages via `RegisterMeowCaller(client)` in `client.go` and `plugins/callplace.go`. This guarantees that the exact `meowcaller.Client` registered during pre-connection startup is stored globally and reused across all call execution handlers (`.groupcall`, `.call`, `.videocall`).
- Adjusted private mode access check in `plugins/dispatch.go`. When the bot is set to `private` mode (`.mode private`), non-sudoer/non-owner command attempts are now silently ignored without sending any text reply.
- Added **Join WhatsApp Support Channel** badge button (`https://whatsapp.com/channel/0029Vb8Vo0k0bIdsTOTF1G2o`) to `README.md`.
- Updated `plugins/callplace.go` with `getMeowCallerClient(ctx.Client)` singleton instance reuse so `.groupcall`, `.call`, and `.videocall` share the exact pre-connected `meowcaller.Client` registered during startup, preventing duplicate client instantiations and raw call adapter errors.
- Enhanced `.menu` command (`plugins/menu.go`) to automatically alternate between plain text output and video media output (sending `resources/songs/whatsrook.gif` as video with menu text caption).
- Fixed `meowcaller` raw call adapter initialization in `client.go`. Instantiating `meowcaller.NewClient` immediately after `whatsmeow.NewClient` (before `client.Connect()`) ensures that low-level `<ack>` and `<call>` hooks are installed prior to the WebSocket connection start, resolving the `"raw call adapter is unavailable: construct the client before connecting whatsmeow"` error.
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
