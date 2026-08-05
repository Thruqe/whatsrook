# WhatsRook Patches & WhatsMeow Overrides

This directory contains custom overrides, patches, and integration scripts for WhatsRook's fork/adapter of `whatsmeow` and `sqlstore`.

## Overview of Custom Modifications

1. **Clean Table Naming (Prefix Removal)**:
   - WhatsRook removes default table prefixes (such as `whatsmeow_` or `meow_`) from the SQL store schema.
   - All tables use clean names (`device`, `identity_keys`, `sessions`, `sender_keys`, `contacts`, `chat_settings`, etc.).

2. **WhatsRook Schema Extensions**:
   - `bot_settings`: Key-value store for session prefixes, bot mode, sudoers, and disabled commands.
   - `call_media_config`: VoIP call audio and video media configurations.
   - `participant_activity`: Tracking active group participant timestamps and message counts.

3. **Audio & PTT Engine**:
   - Automated conversion of audio to 64-bin amplitude Opus OGG voice notes with `PTT = true`.

## Scripts & Tools

- `apply_patches.sh`: Verifies, syncs, and applies WhatsRook's custom patches to the `sqlstore` package.
- `scripts/sync_sqlstore.sh`: Syncs upstream `whatsmeow/store/sqlstore` changes while preserving WhatsRook's prefix-free table schema.
