# Group video and reactions

**Status:** initial group-video signaling and participant H.264 receive accepted live;
multi-PID HBH-FEC relay descriptors KAT-verified, live retest pending

**Reference pinned at:**

- capture SHA-256 `47e4966e1847b686b3a31c4983df8025617d200ec27a71c5884598488af65b90`
- live diagnostic call `14F2B768B13898CC1783AD897C1B5953`

## Observed facts

- Group roster devices carry independent audio, video, and app-data SSRCs.
- The installed group key epoch derives and installs participant-specific receive
  pipelines for all three media kinds.
- Authenticated video and app-data packets can therefore be attributed to the
  participant device selected by their SSRC.
- Captured RTC app-data reaction payloads contain a monotonically increasing
  transaction ID and an emoji. Sender identity comes from the authenticated
  participant media pipeline, not from the protobuf payload.
- Whatsmeow's verified direct-call offer places the H.264 `video` capability after
  the audio capabilities and before `net`.
- On 2026-07-26, initial video group call
  `E9707AFBD7A2DCD45B4BAA54EAB1017F` was accepted by WhatsApp. All three
  participants reached connected PID-backed roster state, the call reached media
  ready with `video=true`, and peers exchanged enabled/orientation video-state
  stanzas. This validates initial group-video signaling and the offer's H.264
  capability placement.
- Outgoing H.264 media did not flow in that run because the web example attempted
  1920×1080 with AVC Baseline Level 3.1. The browser rejected the encoder
  configuration before it produced a frame.
- In subsequent live call `14F2B768B13898CC1783AD897C1B5953`, the relay
  delivered participant H.264 on the participant's slot-2 SSRC with RTP payload
  type 97. Meowcaller authenticated, depacketized, and emitted 42 Annex-B access
  units. The first access unit contains H.264 SPS, PPS, and IDR NAL units.
- WhatsApp Web's captured group-video Allocate contains these attributes in order:
  relay token `0x4000`, sender subscriptions `0x4025`, receiver subscriptions
  `0x4021`, local stream descriptors `0x4024`, participant count `0x805a`, relay
  endpoint `0x0016`, and message integrity `0x0008`.
- The captured sender-subscription protobuf groups the local slot-2/3/5 video
  SSRCs, slot-7/8/6 secondary-video SSRCs, slot-0/1/4 audio SSRCs, and slot-6
  app-data SSRC. Connected remote PIDs 1 and 2 are attached to primary video,
  audio, and app-data; the secondary-video group has no PID targets.
- The captured receiver-subscription protobuf is the ordered pair
  `12 02 08 01 12 02 08 02`, selecting connected remote PIDs 1 and 2. Attribute
  `0x805a` carries `02`, matching the two selected remote participants.
- When two remote participants are connected, the captured `0x4024` value appends
  two descriptors after the nine local media streams. They identify HBH-FEC TX as
  participant/layer `3/3` and HBH-FEC RX as `4/3`. Their SSRCs are the normal
  participant derivations for slot words 7 and 8.
- The captured stack sends a sender/descriptor update without `0x4021`, followed
  59 ms later by the final Allocate containing the receiver subscriptions. The
  final packet is the complete relay state pinned by the KAT.
- Meowcaller's existing group relay refresh only rotates credentials and repeats
  local stream descriptors. It omits all three captured participant-subscription
  attributes. In a later live call, both peers signaled enabled video but the
  relay forwarded no participant PT-97 packets, while audio continued.
- Live call `5B1243C9301A31BD73D76A85B26F2981` accepted the subscription-bearing
  Allocate. With one remote PID selected, participant H.264 flowed. The
  two-remote-PID refresh at `1785024103163` increased the Allocate from 464 to
  476 bytes and was followed by an 11,985 ms gap with no incoming video access
  units while audio continued. When the roster returned to one remote PID, the
  464-byte refresh at `1785024114785` was followed by H.264 resuming on the same
  SSRC 398 ms later.
- The current nine-stream derivation uses slot 6 as the third secondary-video
  SSRC, while the app-data sender independently derives that same slot 6. The
  resulting subscription advertises one SSRC in both the secondary-video and
  app-data groups.
- The authoritative WhatsApp capture does not have that collision. For SELF it
  advertises secondary-video SSRCs `E0E04163 / 74ED8516 / DEA8A613` and the
  distinct app-data SSRC `B31DED3E`. The logs describe the secondary triple as a
  generated video stream and preserve the slot-6-derived app-data SSRC.
- Collision-free live call `7DC5731B5F943349B3E3089BF948D80C` still loses all
  inbound relay RTP when the roster grows from one to two remote participants.
  The last inbound packet is at `1785025792861`; the two-PID 472-byte Allocate is
  sent at `1785025792867`. No inbound RTP arrives for 53,237 ms. After a one-PID
  464-byte Allocate at `1785025846068`, relay RTP resumes 30 ms later and
  authenticated H.264 resumes 469 ms later.
- That failed two-PID Allocate contains only the nine local stream descriptors.
  It omits the captured HBH-FEC TX/RX descriptors required by the complete
  multi-participant relay shape.
- The captured group call sends video state 6 and later returns to state 1 on
  the same call ID. State 6 is therefore a video-to-audio downgrade, not call
  termination; camera-only mute/unmute remains state 0/1.

## Inferences to validate live

- A singular add-person offer for an active video call carries that same `video`
  child and advertises the video capability to the invited device.
- A participant added to a video call joins the existing shared group relay/key
  epoch and receives subsequent group updates rather than negotiating a separate
  media session.
- The missing HBH-FEC TX/RX descriptors cause the relay to stop forwarding when
  it switches from one remote to true multi-participant SFU mode. The wire mismatch
  and stop timestamp are exact, but a descriptor-bearing live retry remains the
  acceptance test.

The remaining inferences must stay marked unvalidated until a live add-person video
call proves the offer is accepted and bidirectional video flows for original and
added participants. A live retest must also prove that adding the captured
participant-subscription attributes makes group-video forwarding deterministic.

## Go envelope

```go
type GroupCallOptions struct {
	GroupJID string
	Video    bool
}

type ParticipantVideoFrame struct {
	ParticipantID string
	Sender        types.JID
	Device        types.JID
	PID           uint32
	HasPID        bool
	SSRC          uint32
	Orientation   int
	AccessUnit    []byte
}

func (c *Call) OnParticipantVideoFrame(func(ParticipantVideoFrame))
```

The existing `ReceiveVideo` sink remains source-compatible and continues receiving
all access units. The participant callback is the group-aware surface and receives
an owned copy so callers cannot retain or mutate the media loop's buffer.

## Web example target

- Start either an audio or video group call from the selector.
- Start a group-bound audio or video call from a numeric group ID or canonical
  `@g.us` JID.
- Coordinate browser capture with call control: start capture for video dial or
  upgrade, stop it after a successful state-6 downgrade, and never reuse hangup
  for a video control.
- Keep one H.264 decoder and canvas per authenticated participant identity.
- Route reactions to the sender's participant tile, with a shared fallback before
  that participant has a tile.
- Preserve the existing direct-call canvas and reaction behavior.

## Validation

- Builder tests must prove exact child order and video capability substitution.
- Call-state tests must prove group calls and video add-person offers retain video
  state.
- Media tests must prove participant metadata and frame bytes survive dispatch
  without aliasing.
- Relay tests must prove a committed connected roster emits the exact captured
  sender/receiver subscription protobufs for remote PIDs, and that the subscription
  update shares the roster/relay atomic commit boundary.
- Relay-stream tests must prove the generated secondary-video SSRCs are nonzero,
  mutually unique, and distinct from audio, primary video, and app-data SSRCs.
- Web tests must prove tagged participant video messages and sender-attributed
  reaction rendering are present, group-ID calls reach the bound API, and
  camera/upgrade/downgrade controls remain independent from hangup.
- Initial group-video signaling and participant H.264 receive are live-accepted.
  The capture-derived subscription protobuf and atomic roster integration KATs
  pass. Live subscription acceptance, bidirectional multi-participant H.264,
  video add-person acceptance, and live group reactions remain pending.
