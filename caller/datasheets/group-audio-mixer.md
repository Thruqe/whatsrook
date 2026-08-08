# Datasheet: `media/group_audio_mixer`

Concurrent, bounded playout for identity-labeled participant PCM produced by the
group receive registry.

**Validation vector:** focused deterministic Go composition KATs plus human-run
live group-call audio validation.

**Reference pinned at:** `UNMAPPED` — the immutable group-call captures prove that
multiple remote participant streams are subscribed concurrently, but they do not
specify PCM gain, clipping, jitter-buffer depth, or sink scheduling. The human
reviewer authorized a live-E2E implementation on 2026-07-24.

## Reference source (verbatim — authoritative)

No authoritative mixer body or audio-mixing vector is available. The neighboring
capture-pinned `media/group_receive` module emits identity-labeled PCM and
deliberately leaves mixing as a separate policy boundary.

The existing 1:1 playout implementation provides the compatibility constraints:

```text
input and output are 16 kHz mono float32 PCM
playout starts after two 960-sample codec frames are buffered
the sink accepts arbitrary-length PCM chunks
one active participant must retain its original sample values
```

The following policy is human-authorized but remains live-E2E unvalidated:

```text
each participant has an independent bounded PCM queue
each participant buffers two codec frames before joining playout
playout advances in 10 ms (160-sample) chunks
simultaneous samples are summed and hard-clamped to [-1, 1]
departed participant queues are removed and cannot be recreated by a late decode
queues keep at most four codec frames and discard oldest audio under backpressure
invite-stage rosters with one remote preserve direct timestamp-aligned playout
the direct buffer drains before switching when a second remote becomes connected
mixed 10 ms chunks are reframed into 960-sample public AudioSink writes
```

## Go envelope (signatures only)

```go
package meowcaller

type participantAudioMixer struct {
	// Internal synchronized participant queues and active-roster gate.
}

type participantAudioSinkFramer struct {
	// Internal 10 ms to public 60 ms frame accumulator.
}

func newParticipantAudioMixer() *participantAudioMixer
func shouldStartParticipantMixing(activeParticipantIDs []string) bool
func (m *participantAudioMixer) Add(participantID string, pcm []float32) bool
func (m *participantAudioMixer) Retain(participantIDs []string)
func (m *participantAudioMixer) MixChunk() ([]float32, bool)
func (f *participantAudioSinkFramer) Push(chunk []float32) ([]float32, bool)
func (p *audioPlayoutBuffer) Drain(sink AudioSink) error
```

The group receive registry additionally exposes its active canonical participant
IDs so a successfully applied roster update can atomically gate future mixer
inserts before deleting departed queues.

## Implementation suggestions (guidance, not authoritative)

- Clone added PCM so decoder-owned buffers cannot be mutated after enqueue.
- Keep one FIFO and prefill state per canonical participant ID.
- Once a roster has been applied, reject `Add` for IDs absent from that roster.
- On overflow, discard the oldest queued samples to preserve real-time behavior.
- Do not average by participant count: a single speaker must retain full gain.
- Clamp the accumulated sample after all ready streams have contributed.
- Keep clocking and sink I/O outside this pure state container so deterministic
  tests do not depend on wall-clock scheduling.
