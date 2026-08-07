# Datasheet: `media/group_key_epoch`

Transaction-wide installation of one decrypted keygen-v2 raw E2E epoch across
the local audio/video/app-data RTP senders, local SRTCP senders, and every
active participant's matching RTP/SRTCP receivers without recreating media
streams or control contexts.

**Validation vectors:** immutable two-sided add-person capture plus focused Go
KATs in `session_test.go`, `group_media_receive_test.go`, and
`engine_lifecycle_test.go`. The raw-root SRTCP cipher/auth/salt constants in
`srtp/e2e_test.go` were independently recomputed with Node's standard
HKDF-SHA256 and AES-128-CTR implementations.

**Reference pinned at:**

- capture SHA-256 `9d6463714430c55ddb3ccb95e153f1d06d11a1feea7a153d1ea95f39f48b6889`
- wacrg group-call crypto commit `4a2d5488b21251303381661aab1ee9bbf4d2cccc`
- Rust raw-key KDF commit `41095d4e6ba4610e054e9ede3af1d5e88a83faee`

This module corrects the participant-scoped interpretation recorded by
`media/group_enc_rekey`. The capture proves that the outer `from` selects the
rekey master/distributor: C distributes transaction 14 to A and B, then A
distributes transaction 16 to C. It does not identify the only media direction
that consumes the new raw key.

## Reference source (verbatim — authoritative)

The immutable two-sided capture report records:

```text
tx14: only C receives group_info rekey="1"
tx14: C sends the same transaction's enc_rekey separately to A and B
tx16: only A receives group_info rekey="1"
tx16: A sends enc_rekey to C
RTP sender SSRC and sequence continuity survive the key transition
```

The user designated the local wacrg specification authoritative. Its group-call
crypto contract states:

```text
A single 32-byte call key is shared with every group-call participant
The same callKey is used by every member; there is no per-pair key
The info label always carries the sender's participant id
Each participant MUST derive its send key from its own normalized id
To receive, it MUST derive a peer's key from that peer's normalized id
```

The pinned Rust keygen-v2 KDF states:

```text
derive_e2e_keys_from_raw(raw_e2e, participant_lid)
  rejects raw_e2e shorter than 32 bytes
  uses raw_e2e[0..32] as HKDF-SHA256 IKM
  uses participant_lid as HKDF info
  derives the RFC 3711 cipher, authentication, and salt keys
```

The authoritative wacrg key schedule also assigns the same participant master
to the distinct SRTCP cipher/authentication/salt labels `0x03`, `0x04`, and
`0x05`. Replacing the keygen-v2 input therefore rotates both the SRTP and SRTCP
branches while retaining the sender's normalized participant ID as HKDF info.

Together these sources establish that one accepted raw epoch must derive:

```text
SRTP/SRTCP send keys = KDF(raw epoch, local normalized device ID)
SRTP/SRTCP recv keys for peer P = KDF(raw epoch, P's normalized device ID)
```

The outer `enc_rekey from` remains signaling metadata for authenticating and
auditing the selected distributor. It is not a media-key ownership selector.

## Go envelope

```go
package srtp

func DeriveE2eSRTCPKeysFromRaw(
	rawE2E []byte,
	participantID string,
) (E2eSrtpKeys, error)
```

```go
package meowcaller

func (p *MediaPipeline) RekeySendFromRaw(rawE2E []byte, selfJID string) error

func (p *MediaPipeline) RekeyRecvFromRawPreservingROC(
	rawE2E []byte,
	peerJID string,
) error

func (r *participantReceiveRegistry) ApplyGroupRawEpoch(
	transactionID uint32,
	rawKey []byte,
) error

func (r *participantReceiveRegistry) UnprotectSRTCP(
	senderSSRC uint32,
	packet []byte,
) ([]byte, uint32, bool)

func (r *participantReceiveRegistry) UnprotectVideo(
	packet []byte,
) (unprotectedParticipantMedia, bool)

func (r *participantReceiveRegistry) UnprotectAppData(
	packet []byte,
) (unprotectedParticipantMedia, bool)
```

The live engine installs one accepted epoch into all attached RTP and SRTCP
senders and the participant receive registry as one operation. Any RTP or SRTCP
sender attached after an accepted epoch immediately inherits the installed
epoch before it can emit a packet. The signaling author is retained in
diagnostics only.

## Required state machine

```text
GroupUpdate(U), U <= rosterTx:
  ignore

GroupUpdate(U), U > rosterTx:
  atomically replace the active connected PID-bearing device indexes
  preserve same-device receiver, decoder, RTP, and authenticated ROC objects
  remove departed receivers
  install the newest buffered epoch K where K <= U
  keep future buffered epochs

EncRekey(K), no roster or K > rosterTx:
  buffer one transaction-wide raw epoch by K

EncRekey(K), roster exists and K <= rosterTx:
  K < installedTx: ignore
  K == installedTx and bytes equal: no-op
  K == installedTx and bytes differ: reject
  K > installedTx:
    derive every active receive key before mutating any pipeline
    derive local and per-participant SRTP and SRTCP keys
    install the raw epoch into every active audio/video/app-data RTP receiver
    and participant SRTCP receiver
    install the same raw epoch into the local RTP/SRTCP senders
    record K only after every derivation and installation succeeds

Call end:
  discard pending and installed epochs with the call
```

An update may remove every remote receiver. The sender still adopts the accepted
epoch so that a subsequently connected participant receives media under the
current call key.

## Continuity and concurrency requirements

- Rekeying must not recreate any attached RTP stream.
- The next outbound packet keeps the same SSRC and advances the existing
  sequence/timestamp/counter state.
- The sender ROC is preserved because the packet sequence space is preserved.
- Each receiver ROC is preserved because the capture shows a key change over a
  continuing RTP stream; resetting the estimator would make sequence rollover
  depend on the key boundary rather than the stream boundary.
- Send protection and send-key replacement must share one mutex so a packet
  cannot combine a key from one epoch with ROC/stream state from another.
- Receive authentication and receive-key replacement must share the existing
  receive mutex.
- SRTCP sender key replacement must share the sender-report mutex so the SRTCP
  index and randomized CNAME survive the epoch transition.
- Incoming SRTCP must exact-route through the active roster using any of the
  participant's nine deterministic relay-stream SSRCs. It must never try every
  participant key.
- Incoming video and app-data RTP must exact-route through the active roster's
  deterministic slot-2 and slot-6 SSRCs. Each participant keeps independent
  receive ROC, video reassembly, and app-data transaction state.
- Derive all replacement keys before installing any of them; a malformed raw key
  must leave the working epoch untouched.

## Validation boundaries

- A packet under the new raw epoch must fail before installation and authenticate
  after installation for every active receiver.
- A packet emitted after installation must authenticate only under the new raw
  epoch derived with the local participant ID.
- A local SRTCP report emitted after installation must authenticate only under
  the raw-root SRTCP labels and continue its pre-transition SRTCP index.
- SRTCP from the original and added participants must authenticate under each
  sender's participant-derived key; unknown and departed SSRCs must be rejected.
- A late-attached local video SRTCP sender must inherit the installed epoch.
- Existing and late-attached local video/app-data RTP senders must authenticate
  only under the installed raw epoch.
- Raw-epoch video and app-data from each active participant must authenticate
  through that participant's independent receive context; old-key, unknown, and
  departed streams must be rejected.
- The outbound packet immediately after installation must preserve SSRC and
  continue sequence/timestamp state.
- Receive ROC state must continue across installation and authenticate the next
  post-wrap packet.
- Rekey-before-roster and roster-before-rekey must converge on the same
  transaction-wide epoch.
- Stale, identical duplicate, conflicting duplicate, malformed-key, future,
  departure, and call-cleanup cases must be covered.
- Ordinary 1:1 media remains on its original call key unless a typed group rekey
  event is received.
- Live Signal encryption/decryption and WhatsApp acceptance remain end-to-end
  boundaries outside this module.

## Evidence boundary

The capture explicitly correlates each raw key event with
`update_self_participant_keys` / `self_srtp_set`, and packet evidence proves the
audio RTP transition. It also records RTCP traffic and SRTCP rejection counters,
but does not correlate an individual SRTCP ciphertext with transaction 14 or 16.
Applying the raw root to SRTCP is therefore grounded in the user-designated
authoritative wacrg key schedule rather than claimed as packet-level proof from
this capture. Live group-SRTCP acceptance remains an end-to-end validation
boundary. The participant-wide KDF and deterministic stream bundle support
applying the epoch to video and app-data RTP as well, but the capture's
packet-level proof is audio-only; live group video/reaction acceptance remains
explicitly pending.
