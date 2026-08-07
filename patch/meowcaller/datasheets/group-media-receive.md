# Datasheet: `media/group_receive`

Participant-indexed primary-audio receive state for direct calls upgraded in place
to ad-hoc group calls.

**Validation vector:** focused synthetic Go KATs composed from the immutable capture
constraints below and the existing byte-exact SSRC/SRTP/WARP KATs.

**Reference pinned at:**

- capture SHA-256 `d565e26f2ca48483525c5bf4dcc2c1bf5ae616299190d128f5f35f74ac50d6c6`
- capture SHA-256 `a91028746497b58d962f14fe5ed4d8036f3ca1c7f2091af5caa52f8430947def`
- whatsmeow commit `7dc1db147f07af4a7b8878a4823e516386547164`
- authenticated media receive commit `2c358ea5dfdf2e9a17908790cc57d31d174dfbd6`

The captures were approved by the human reviewer as authoritative on 2026-07-23.

## Reference source (verbatim — authoritative)

The immutable raw JSONL files remain the verbatim authority. The compact reports
are `diag/analysis/group-call-add-people-v2-20260723-112208.md` and
`diag/analysis/group-call-multi-add-v2-20260723-135301.md`.

```text
the direct-call ID remains unchanged during the ad-hoc group upgrade
the initial direct relay ACK assigns the local endpoint self_pid 1
transactions 8 and 10 retain the local endpoint and original peer as connected
without assigning a PID to any remote device in those transitional rosters
the original peer remains connected and becomes PID 0
the local participant becomes PID 1
the connected added participant becomes PID 2
only connected winning devices carry PIDs and become active media members
pending receipt-state devices can have reserved SSRCs but no active media
the original peer's primary-audio SSRC and sequence progression remain continuous
the added participant uses its preallocated device-specific primary-audio SSRC
both remote participants are simultaneously subscribed after the group update
participant media/key state is independent and removed with active membership
```

The receive invariant derived from that ordering is explicit: a local/self PID is
not evidence that remote PID routing is ready. The authenticated direct receiver
remains active until a connected remote device carries a PID.

The existing verified helpers define the byte-level composition:

```text
canonical device ID: rtp.FormatE2ESrtpParticipantID(device JID)
primary audio SSRC: rtp.DeriveWasmParticipantSsrc(call ID, device ID, slot 0)
participant receive keys: srtp.DeriveE2eKeys(callKey, device ID)
packet acceptance: authenticated MediaPipeline.UnprotectAudio
```

## Go envelope (signatures only)

```go
package meowcaller

type participantAudioDecoder interface {
	Decode([]byte) []float32
}

type decodedParticipantAudio struct {
	ParticipantID string
	UserJID   types.JID
	DeviceJID types.JID
	PID       uint32
	HasPID    bool
	SSRC      uint32
	Timestamp uint32
	PCM       []float32
}

type participantReceiveRegistry struct {
	// Internal synchronized participant/device/PID/SSRC state.
}

func newParticipantReceiveRegistry(
	callID string,
	callKey []byte,
	selfLID string,
	peerLID string,
	decoderFactory func() participantAudioDecoder,
	opts ...Option,
) (*participantReceiveRegistry, error)

func (r *participantReceiveRegistry) ApplyGroupUpdate(update types.GroupCallUpdate) error
func (r *participantReceiveRegistry) ApplyGroupUpdateTransaction(
	update types.GroupCallUpdate,
	apply func(commit func()) error,
) error
func (r *participantReceiveRegistry) HasCommittedGroupUpdate() bool
func (r *participantReceiveRegistry) RekeyFallback(peerLID string) error
func (r *participantReceiveRegistry) ActiveParticipantIDs() []string
func (r *participantReceiveRegistry) DecodeAudio(packet []byte) (decodedParticipantAudio, bool)
```

`engine.install` consumes `events.CallGroupUpdate`. An update received before the
media loop starts is retained on the `engineCall`; once the registry exists it is
applied through an installed callback. Updates do not replace `Call.Peer()`.

## Cross-resource commit boundary

One accepted WhatsApp group transaction drives both the active receiver/key
snapshot and the rotated relay allocation. Those resources form one media commit:

```text
stale roster transaction:
  do not invoke the external apply callback
  change neither receiver/key state nor relay state

new roster transaction:
  validate every participant, device, PID, and derived SSRC
  construct every next receiver index
  select the newest eligible pending raw epoch
  derive every next send, receive, and SRTCP key
  invoke the external apply callback while the receiver snapshot is still locked

external failure before commit:
  discard the prospective indexes and derived keys
  preserve the prior receiver transaction, maps, metadata, installed epoch,
  pending epochs, RTCP reception set, and relay allocation

external success:
  invoke the supplied commit closure exactly once after the last fallible
  transport operation
  install the already-derived keys and swap the receiver indexes without another
  error return
  publish the relay allocation before releasing its lock
  only then expose the new roster transaction to the engine
```

The commit closure is deliberately synchronous and non-failing. The live relay
path invokes it from the successful Allocate send callback, while the relay state
lock is still held and before the relay credentials are published. Registry
readers remain blocked until that relay operation returns. This establishes one
linearization boundary: readers can observe either the complete prior media
snapshot or the complete accepted media snapshot, never a receiver-only or
relay-only accepted update.

`ApplyGroupUpdate` remains the receiver-only compatibility surface and delegates
to the same transaction with an immediate commit. No ordinary 1:1 path uses the
transactional group surface.

## Implementation suggestions (guidance, not authoritative)

- Seed one 1:1 fallback receiver from `CallMediaReady.PeerLID`.
- Advance transaction state but preserve the authenticated direct receiver when
  a transitional group snapshot has no connected remote PID-bearing devices.
  A PID on the local/self device does not retire the remote fallback.
- Activate only devices whose parent participant state is `connected` and whose
  device has `HasPID`; PID zero is valid.
- Exclude the local device by canonical participant ID.
- Reuse receivers by canonical device ID so the original peer's ROC and decoder
  state survive promotion to PID zero.
- Build the next PID/SSRC indexes completely, then swap them under the registry
  lock; pending candidates never enter an active index.
- Separate prospective raw-epoch derivation from installation. The prospective
  phase may return an error; the commit phase only assigns already-validated
  state and installs already-derived fixed-size key values.
- Hold the registry lock from prospective validation through external apply and
  commit. When relay rotation is required, invoke the registry commit closure
  only after the Allocate write succeeds and while the relay-state lock is still
  held.
- Zero prospective raw-key copies and derived key structs after both rollback and
  successful installation. Consume and zero buffered epochs only at commit.
- Route by expected SSRC before crypto and decoder work.
- Give every device its own `MediaPipeline` and decoder.
- Remove departed receivers immediately. A packet/key grace window is not proven
  by the captures.
- Emit identity-labeled decoded frames. Mixing is a separate module because RTP
  origins, gain, clipping, jitter, and sink backpressure are not specified by the
  captures.
