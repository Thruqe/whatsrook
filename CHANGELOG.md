# Changelog

All notable changes to the WhatsRook project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

- YouTube Video & Audio Download Commands (`plugins/youtube.go`, `downloader/youtube.go`):
  - Implemented `.ytv` (YouTube Video) and `.yta` (YouTube Audio with `ffmpeg` audio encoding for WhatsApp voice/audio playback).
  - Derived YouTube downloader engine from Embers session proxy (`https://downr.org/.netlify/functions/nyt`) with automatic session cookie initialization and retries.

- Hidden Bot Customization Suite & Emoji Cleanup (`plugins/botname.go`, `plugins/dispatch.go`):
  - Hidden `.setbot` / `.bot` command from public `.menu` listing (`IsPublic: false`), operating as a standalone onboarding setup suite triggered on default bot name detection.
  - Removed decorative emojis across `.setbot` wizard and default name recommendation prompts.

- Unified Bot Customization Suite & Guided Setup Wizard (`plugins/botname.go`, `events.go`, `plugins/menu.go`):
  - Created `.setbot` / `.bot` command suite for configuring Bot Name, Menu Thumbnail, Command Prefix, and Status Bio.
  - Implemented 4-step guided setup wizard (`.setbot wizard`). Steps 1 & 2 (Bot Name & Thumbnail) are required, after which a `Skip ⏭️` button is unlocked for Steps 3 & 4 (Prefix & Bio).
  - Removed standalone `Customize` button from `.menu` and integrated thumbnail customization into `.setbot`.

- TikTok Extractor Upgrade (`downloader/tiktok.go`):
  - Integrated high-speed TikWM API as Strategy 1 to bypass TikTok HTML rehydration changes and anti-bot challenges, resolving extraction errors for TikTok video posts (e.g. ID `7453503456137973000`).

- Interactive Media Downloader & URL Validation (`plugins/downloader.go`, `downloader/downloader.go`, `events.go`):
  - Created a 5-page interactive `.dl` menu showing 3 social media services per page with `◀️ Prev` and `Next ▶️` pagination buttons.
  - Implemented interactive service selection (`.dl prompt <service>`) that prompts users to drop a URL, then listens and automatically downloads the media upon link receipt.
  - Added strict URL validation helpers (`downloader.ValidateURL` / `downloader.IsValidURL`) to prevent malformed or unsupported URL processing.

- Menu Video GIF Playback Fix (`resources/songs/whatsrook.mp4`, `plugins/menu.go`, `sender/abstract.go`):
  - Converted `resources/songs/whatsrook.gif` to `resources/songs/whatsrook.mp4` using `ffmpeg` (H.264 / yuv420p video container required by WhatsApp for inline GIF playback).
  - Updated `.menu` command to load `whatsrook.mp4` and dispatch via `ReplyWithVideoGif`.
  - Standardized all Protobuf field pointers to Go `new(...)` pointer initializer syntax.

- Downloader Command Update (`plugins/downloader.go`):
  - Updated `.dl` / `.downloader` command usage response to list all 13 supported media services (Facebook, Instagram, Twitter/X, TikTok, Snapchat, Reddit, Pinterest, SoundCloud, Threads, Bluesky, VK, Tumblr, Twitch).

- Bot Bio Command (`plugins/bio.go`):
  - Added `.bio` / `.setbio` command for bot owner and sudoers to update WhatsApp status bio on demand.

- ViewOnce Customization & Destination Routing (`plugins/chats.go`):
  - Added `.vv customize` guide and destination selector (`.vv dest chat | owner | <phone_number> | <group_jid>`) allowing users to configure where unwrapped ViewOnce media is delivered.

- Status Reaction Feature (`plugins/likestatus.go`, `events.go`):
  - Added `.likestatus` command with 2-button interactive flow.
  - Automatically reacts to incoming WhatsApp status broadcasts with a random love emoji.

- Command Renaming & Cleanups (`plugins/sudo.go`, `plugins/events_cmd.go`, `events.go`):
  - Renamed `autostatussave` command to `autostatus` (with `statussave`, `statusauto`, `autostatussave` aliases).
  - Updated `autovv` and `autostatus` to use 2-button interactive control flow.
  - Removed decorative emojis from `.events` group event notification messages for cleaner formatting.

- Warning System & Automated Enforcement (`plugins/warn.go`):
  - Implemented `.warn`, `.unwarn`, `.warns`, and `.setwarn` commands with configurable warning thresholds.
  - Enforces automated blocking & group kicking upon reaching warning limit (respects admin immunity rules unless bot is group owner, blocks sender in DMs).

- Real-Time Group Events Listener & Command (`plugins/events_cmd.go`, `events.go`):
  - Implemented `.events` command with interactive 2-button control flow.
  - Listens to `whatsmeow` `*events.GroupInfo` updates and broadcasts real-time group event alerts (Subject changes, Description updates, Settings lock/unlock, Announce mute/unmute, Admin promotions/demotions).

- Events Helper Refactoring (`utils/events_helper.go`, `events.go`):
  - Moved message text extraction and media type identification routines (`ExtractMessageText`, `GetMediaType`, `ExtractTextFromProto`) to `utils/events_helper.go`.

- Command Category Cleanup & Simplified Interactive Settings (`plugins/`):
  - Normalized command categories across all plugins (`ai`, `calls`, `chats`, `downloader`, `filters`, `games`, `group`, `info`, `media`, `misc`, `owner`, `settings`, `tools`).
  - Simplified `welcome`, `goodbye`, `antispam`, `antimsg`, `anticall`, and `autobio` to present 2 interactive buttons upon invocation: a dynamic `Activate`/`Deactivate` toggle button based on current state, and a `Customize` button.
  - Implemented `.customize` sub-command guides for all toggleable settings that output available options, placeholders (`{user}`, `{group}`, `{desc}`), and concrete usage examples.

- Multi-Service Media Downloader (`downloader/`, `plugins/downloader.go`):
  - Created package `downloader` with extractors for Facebook, Instagram, Twitter/X, TikTok, Snapchat, Reddit, Pinterest, and SoundCloud inspired by Cobalt engine patterns.
  - Implemented `.dl`, `.facebook` (`.fb`), `.instagram` (`.ig`), `.twitter` (`.x`/`.twt`), `.tiktok` (`.tt`), `.snapchat` (`.snap`), `.reddit` (`.rd`), `.pinterest` (`.pin`), and `.soundcloud` (`.sc`) commands.

- Security Fix for Archive Extraction (`updater/updater.go`):
  - Fixed a Zip/Tar Slip vulnerability during release archive extraction by enforcing strict path sanitization (`filepath.Clean`) and verifying extracted file paths remain strictly inside the destination directory.

- Pterodactyl Deployment Panel Script (`scripts/panel.zip`, `scripts/Whatsrook panel deployment.zip/index.js`):
  - Extracted and updated `index.js` parsing logic to support the new `log/slog` PAIR CODE output format from `connection.go` (`slog.Info("PAIR CODE: %s\n", "", code)`).
  - Handles multi-line log output splitting, strips ANSI color codes, and maintains backward compatibility with legacy log formats.
  - Re-packaged updated files into `scripts/panel.zip`.

- Go Web Client & Daemon Bridge (`example/web/main.go`, `example/web/README.md`):
  - Converted `example/web` from Bun.js/TypeScript to a pure Go web client application.
  - Implements the exact 5-screen interactive dashboard (phone input, QR code scan, 8-character pair code, live status dashboard, activity feed, logout).
  - Built using `github.com/Thruqe/htmlbuilder` for UI construction and `github.com/coder/websocket` to bridge browser WebSockets with the WhatsRook daemon's Protobuf WebSocket interface.

### Removed

- Makefile References in Agent Prompts & Docs (`GEMINI.md`, `CLAUDE.md`, `Documentation/README.md`):
  - Removed all Makefile references and replaced them with standard Go toolchain commands (`go build`, `go test ./...`, `./scripts/generate-proto.sh`).

- Complete Removal of Ember Package & Ember Commands (`ember/`, `plugins/play.go`, `plugins/fetch.go`, `plugins/facebook.go`, `plugins/instagram.go`, `plugins/tiktok.go`, `plugins/twitter.go`, `plugins/threads.go`, `plugins/cookie.go`):
  - Completely deleted the `ember` package (`ember/ember.go`, `ember/ember_test.go`).
  - Removed all commands relying on the Ember API: `play`, `ytv`, `yta`, `dl`, `download`, `fetch`, `facebook`, `fb`, `instagram`, `ig`, `tiktok`, `tt`, `twitter`, `x`, `twt`, `threads`, `th`, `cookie`, `cookies`, `setcookie`.
  - Cleaned up all Ember dependencies in `sender/send.go`, `events.go`, and `plugins/helper.go`.

- Animated Task Loader Feature (`sender/loader.go`, `sender/loader_test.go`, `sender/abstract.go`):
  - Implemented `ctx.StartLoader(text)` which sends an initial status reply and continuously edits the message with an animated Braille spinner (`⠋`, `⠙`, `⠹`, `⠸`...) every 1.2s while a command executes.
  - Integrated animated loaders across all commands (`play`, `ytv`, `yta`, `dl`, `facebook`, `instagram`, `tiktok`, `twitter`, `threads`, `ai`, `movie`, `news`, `sticker`, `circle`, `crop`, `steal`, `mp4`, `mp3`, `mp4url`, `black`, `sh`, `ping`, `cpu`, `memory`, `status`, `update`, `report`, etc.).
  - Added `ReplyWithID` helper method to `sender.Context` and unit test `TestLoaderLifecycle`.
- Per-Platform Cookie Support & Embers Sync (`plugins/cookie.go`, `plugins/helper.go`):
  - Updated `setcookie` command syntax to `setcookie PLATFORM <netscape_cookie>` (e.g. `setcookie YOUTUBE <cookies>`, `setcookie TWITTER <cookies>`).
  - Automatically posts Netscape cookies to Embers API `/cookies` endpoint for caching.
  - Removed 10-minute cookie expiration notice as requested.
- `ytv` and `yta` Commands & Interactive Quality Selection (`plugins/play.go`):
  - Added dedicated `ytv` (video) and `yta` (audio) commands for YouTube downloads.
  - Implemented interactive buttons with quality choices (e.g., `360p`, `720p`, `1080p` for video; `128kbps`, `320kbps` for audio) without emojis.
  - Refactored `play` command to present interactive quality selection buttons.
- On-Demand Cookie Prompting for Non-YouTube Downloads (`plugins/fetch.go`, `utils/utils.go`):
  - Enforced YouTube-only default cookie passing; non-YouTube downloads (Twitter/X, Instagram, TikTok, Facebook, Threads) only request cookies when an authentication or login error occurs.
- Emoji Removal from `autoacceptcall` (`plugins/autoacceptcall.go`):
  - Cleaned up all status and command response messages to remove emojis.

### Security

- Erased `cookie.txt` from Git history via `git filter-branch` and updated `.gitignore` (`cookie.txt`, `*.cookie`, `*.txt`) to prevent sensitive credential files from being tracked.

### Changed

- Download Format Exception Log Adjustments (`ember/ember.go`, `plugins/play.go`):
  - Changed log severity for external Embers/yt-dlp extraction errors (e.g. format restriction errors) from `ERROR` to `WARN` so user-driven YouTube download failures do not pollute `debug.log`.
- Error-Only Log Filtering for `debug.log` (`logger/logger.go`, `logger/logger_test.go`):
  - Adjusted logger configuration across `slog`, `zerolog`, and whatsmeow adapters so `debug.log` strictly captures `ERROR` level events.
  - Console output (stdout) retains full log output based on verbosity (`INFO` / `DEBUG`).
  - Added unit test `TestDebugLogOnlyErrors` in `logger/logger_test.go`.
- Ember API Integration Rewrite (`ember/ember.go`, `plugins/play.go`):
  - Replaced local `yt-dlp` search in `play`/`ytv`/`yta` commands with Ember's `/youtube/search` endpoint; removed `go-ytdlp` dependency.
  - `ember.Fetch` no longer passes raw cookie content as a query parameter — cookies are managed server-side by Embers (registered via POST `/cookies` from `setcookie`).
  - Added `ember.SearchYouTube()` function wrapping the `/youtube/search` API.
  - Interactive quality buttons now embed only the YouTube video ID (not full URL) to keep button IDs compact and valid.
  - `handlePlayDownload` now correctly falls back to top-level stream URL for audio-only mode when no dedicated audio media entry is present.
  - Increased Ember HTTP client timeout from 30s to 90s to handle long yt-dlp extraction.
  - Removed unused `isCookieError` function; renamed to `isCookieError2` (takes pre-lowercased string for efficiency).
  - `go mod tidy` cleaned `go-ytdlp` from `go.mod`/`go.sum`.


- AutoAcceptCall Forced Accept Protocol Deadlock & Duplicate Accept Prevention (`plugins/autoacceptcall.go`, `plugins/callplace.go`):
  - Solved incoming call deadlock with WhatsApp Android callers (`stop_probing_before_accept_send=1`) by sending forced `<accept>` node via `DangerousInternals` after relay election.
  - Added `clearMeowcallerAcceptPending` using reflection and `unsafe.Pointer` to clear `acceptPending` on `meowcaller.engine`, preventing duplicate `<accept>` nodes when `<mute_v2>` arrives and eliminating call reconnecting/disruption.
  - Enabled `meowLogger()` in `plugins/callplace.go` for real-time visibility into `meowcaller` relay and media pipeline events.

- AutoAcceptCall OnReady Pipeline Sync & AntiCall Bypass Fix (`plugins/autoacceptcall.go`, `events.go`):
  - Updated `events.go` to skip `handleAntiCall` when `autoacceptcall` status is `on`, preventing conflict between call rejection and call answering.
  - Registered `call.OnReady` callback before calling `call.Answer()` in `HandleIncomingCallAutoAccept`, ensuring audio and video media playback begins strictly after `meowcaller` establishes the active VoIP transport.
- `meowcaller` Inbound Call Architecture Alignment (`plugins/callplace.go`, `plugins/autoacceptcall.go`):
  - Aligned incoming call handling strictly with `meowcaller` specification: registered `HandleIncomingCallAutoAccept` handler via `mc.OnIncomingCall` on `meowcaller.Client` initialization.
  - Ensures `meowcaller` receives incoming call offers, sends `<preaccept>` eagerly, invokes the `OnIncomingCall` callback, answers via `call.Answer()`, streams configured audio/video media, and hangs up cleanly when completed.
- Call Media JID & LID Lookup Resolution (`store/sqlstore/callmedia.go`):
  - Updated `GetCallMediaConfig` to attempt exact JID matching, user ID matching (handling `@lid` vs `@s.whatsapp.net` JID variations), and fallback to the latest saved media for `our_jid`.
  - Fixes `autoacceptcall: enabled but missing required call media audio= video=` issue when media is configured under an `@lid` JID.
- `setvideocall` Command & Status Overview (`plugins/videocall.go`):
  - Registered `.setvideocall` (`setvc`, `setvcall`, `setvideocallaudio`, `setvideocallmedia`) command to save default video media for video calling and auto-accept calls.
  - Downloads, converts, and saves attached/quoted video files; displays current saved video file status when invoked without attachments.
- AutoAcceptCall Command & Automatic Media Answer Handling (`plugins/autoacceptcall.go`, `events.go`):
  - Created `.autoacceptcall` (`.autoaccept`, `.acceptcall`) command (`on`, `off`, `status`).
  - Enforces prerequisite validation: requires the user to have configured both call audio (`.callaudio`) and call video (`.videocall`) media before enabling auto-accept.
  - Integrated `HandleAutoAcceptIncomingCall` into `events.go` `*events.CallOffer` handler: automatically answers incoming voice or video calls using `meowcaller`, streams saved call audio/video media to the caller, and hangs up when playback completes.
- Dynamic Bot Name `{NAME}` Template Placeholder in Meta AI System Prompt (`prompts/meta_ai.txt`, `meta/parser.go`, `meta/cache.go`):
  - Updated `prompts/meta_ai.txt` to use `{NAME}` placeholder for the bot identity prompt (`Your name is {NAME}...`).
  - Added `{NAME}` string replacement in `meta.BuildRunCommandInstructionWithName` to dynamically inject the user-configured bot display name into the Meta AI system prompt.
  - Added instruction cache invalidation (`meta.ClearInstructionCache()` / `GetOrBuildInstructionWithName`) so AI system prompt instantly updates whenever the bot name is modified via `.botname`.
- Bot Name Setup Scoped Strictly to Triggered Commands (`plugins/dispatch.go`):
  - Refactored default bot name setup interception so it triggers ONLY when an actual bot command is invoked (e.g. `.ping`, `.menu`, `rook play`).
  - Ensures regular group/DM casual chatting is never interrupted by setup prompts.
- Connection Notification Owner DM Mention (`events.go`):
  - Updated `notifyOwnerConnected` to parse owner JID, mention `@owner` in the notification text, and populate `ContextInfo.MentionedJID` array so WhatsApp client highlights the mention.
- Pre-Execution Default Bot Name Interception Fix (`plugins/dispatch.go`):
  - Moved default bot name setup interception (`WhatsRook`, `whatsrook`, `rook`) to run before any command or handler execution in `Dispatch`.
  - Added support for all bot name command aliases (`botname`, `setbotname`, `setname`, `name`) so name setup commands bypass interception while all other commands trigger the setup prompt until customized or ignored.
- Uncustomized Bot Name Interactive Setup Flow (`plugins/botname.go`, `plugins/dispatch.go`):
  - Intercepts incoming commands when the bot name is uncustomized / default (`WhatsRook`, `whatsrook`, `rook`).
  - Displays interactive buttons: `"It's highly recommended to give your own copy of WhatsRook its own name... [Customize Bot] [Continue]"`.
  - Selecting `"Continue"` issues a second warning message: `"Please note: keeping the bot name as WhatsRook will have its consequences... [Customize Bot] [Ignore]"`.
  - Selecting `"Ignore"` saves user preference and unlocks normal command execution.
  - Selecting `"Customize Bot"` prompts for the new bot name, updates `bot_name` in DB upon response, and informs the user they can change it anytime later using `[prefix]botname`.
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
