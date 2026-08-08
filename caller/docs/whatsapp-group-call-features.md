# WhatsApp group-call feature surface

This branch keeps meowcaller compatible with the latest upstream
`go.mau.fi/whatsmeow`. It does not require the Titan fork.

Whatsmeow still owns the transport connection and normal call events. Group-call,
call-link, waiting-room, hand, and screen-share actions that are not yet exposed
by upstream whatsmeow are encoded as WhatsApp binary nodes and sent through
`Client.DangerousInternals()`. For incoming actions, the adapter wraps
whatsmeow's private `call` node handler through reflection. Recognized group
children are parsed and acknowledged once by meowcaller; every unrecognized
node is delegated to the original upstream handler. The existing 1:1 signaling
path remains unchanged.

## Starting and extending calls

```go
call, err := client.GroupCallWithOptions(
    ctx,
    []string{"+15550000001", "+15550000002"},
    meowcaller.GroupCallOptions{Video: true},
)

// Bind the call to a WhatsApp group and select all remote group members:
call, err = client.GroupCallByIDWithOptions(
    ctx,
    "1234567890-1234567890@g.us",
    meowcaller.GroupCallOptions{Video: true},
)

err = call.AddParticipant(ctx, "+15550000003")
err = call.RingParticipant(ctx, "+15550000004")
```

`GroupState` and `OnGroupState` expose transaction-ordered participant/device
state without relay credentials, key material, or raw capability blobs.
`RingParticipant` is only for a non-connected user already present in the
authoritative roster; `AddParticipant` adds a new user.

An ordinary 1:1 call may become an ad-hoc group call through `AddParticipant`.
An invited endpoint receives the active call as an incoming group call and uses
the normal `Answer` or `Reject` methods.

## Reactions, hand state, screen sharing, and video

```go
call.SendReaction("🫡") // arbitrary Unicode emoji; empty clears it
call.SetHandRaised(true)
call.StartScreenShare(nil)
call.StopScreenShare()

call.OnReaction(func(r meowcaller.CallReaction) {})
call.OnHandRaise(func(s meowcaller.HandRaiseState) {})
call.OnScreenShare(func(s meowcaller.ScreenShareState) {})
call.OnParticipantVideoFrame(func(f meowcaller.ParticipantVideoFrame) {})
```

Screen sharing is a signaling/source-role change, not an independent media
sender. In the captured non-dual-stream mode, camera → display → camera uses the
same primary video SSRC and preserves RTP sequence continuity. A browser should:

1. enable or upgrade video if necessary;
2. send `StartScreenShare` before display frames enter `SendVideo`;
3. switch the local H.264 source and request a fresh IDR;
4. send `StopScreenShare`, restore the camera source, and request another IDR.

Stopping screen share or local video does not hang up the call. Camera and
remote-video directions remain independent.

## Call links and waiting rooms

```go
link, err := client.CreateCallLink(ctx, meowcaller.CallLinkOptions{Video: true})
preview, err := client.PreviewCallLink(ctx, link.URL, meowcaller.CallLinkOptions{Video: true})
call, err := client.JoinCallLink(ctx, link.URL, meowcaller.CallLinkOptions{Video: true})

call.SetApprovalRequired(ctx, true)
call.OnWaitingRoomState(func(state meowcaller.WaitingRoomState) {
    for _, user := range state.Users {
        _ = call.AdmitParticipant(ctx, user.JID.String())
        // Or: call.DenyParticipant(ctx, user.JID.String())
    }
})
```

A join that requires approval remains in `CallPhaseWaitingRoom`. Meowcaller
sends the captured waiting-room heartbeat and does not start relay media until
an authoritative admitted group snapshot arrives.

There is no separate admit-all, link revocation, or link-expiry API in the
captured product flow. Scheduling a call is a WhatsApp event-message feature
that embeds a normal call link; it belongs in the messaging layer rather than
the media library.

## Validation boundary

The raw stanza builders/parsers, transaction ordering, shared group-key epochs,
participant SRTP/SRTCP routing, relay subscription packets, audio mixing,
multi-participant H.264/reactions, and `0x09` relay-forwarding envelopes have
capture-derived tests. This branch remains experimental until fresh live calls
validate the full upstream-whatsmeow adapter end to end.

The browser console under `examples/web` is the intended live test harness.
Diagnostic recordings contain secrets and media; keep them local.
