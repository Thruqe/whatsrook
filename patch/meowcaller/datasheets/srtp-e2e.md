<!-- Datasheet = three things only: the reference source VERBATIM, the Go envelope
     (signatures, no bodies), and implementation suggestions. No behavioral summary,
     no implementation. The verbatim source is the only authoritative content. -->

# Datasheet: `srtp/e2e` receive ROC

Receive-side rollover estimation and authenticated state commit.

**Validation vector:** `srtp/e2e_test.go` — wrap/reorder tracking and the
unauthenticated two-packet staircase from the pinned reference.

**Reference pinned at:** `2f001b5a3d6374cc5cf7177792c2a81f87a54080`

## Reference source (verbatim — authoritative excerpt)

```rust
/// Recv-side ROC estimator (RFC 3711 §3.3.1 guess-index). Unlike the monotonic send tracker it
/// tolerates reorder/loss: each packet's ROC is guessed from the highest seq seen, so a late
/// packet straddling a wrap decrypts under the right (lower) ROC without poisoning the state.
#[derive(Default)]
pub(crate) struct RecvRocTracker {
    roc: u32,
    s_l: u16,
    initialized: bool,
}

impl RecvRocTracker {
    /// Estimate the ROC for `seq` WITHOUT mutating state (RFC 3711 guess-index).
    /// Use this to build the IV / verify a packet; only [`Self::commit_roc`] after
    /// the packet authenticates, so an unauthenticated packet can't desync state.
    pub fn estimate_roc(&self, seq: u16) -> u32 {
        if !self.initialized {
            return self.roc;
        }
        // Pick v in {roc-1, roc, roc+1} so 2^16*v+seq is closest to 2^16*roc+s_l. The signed 16-bit
        // gap (not a modular wrapping_sub) is what distinguishes "next-but-reordered" from "wrapped".
        if self.s_l < 0x8000 {
            if (seq as i32 - self.s_l as i32) > 0x8000 {
                self.roc.wrapping_sub(1) // old packet from before the origin (roc-1)
            } else {
                self.roc
            }
        } else if (self.s_l as i32 - seq as i32) > 0x8000 {
            self.roc.wrapping_add(1) // forward wrap into roc+1
        } else {
            self.roc
        }
    }

    /// Fold an AUTHENTICATED packet's `(v, seq)` into the state; `v` must come from
    /// a prior [`Self::estimate_roc`] whose MI tag verified. Seeds from the first
    /// packet (roc stays 0).
    pub fn commit_roc(&mut self, v: u32, seq: u16) {
        if !self.initialized {
            self.s_l = seq;
            self.initialized = true;
            return;
        }
        if v == self.roc {
            if seq > self.s_l {
                self.s_l = seq;
            }
        } else if v == self.roc.wrapping_add(1) {
            self.roc = v;
            self.s_l = seq;
        }
        // v == roc-1 (reordered late packet): leave state untouched.
    }

    /// Estimate + commit in one step. Test-only: production authenticates the WARP
    /// MI tag against `estimate_roc` first and only `commit_roc` on success, so an
    /// unauthenticated packet can't fold state. Kept for the wrap-tracking tests.
    #[cfg(test)]
    pub fn guess_roc(&mut self, seq: u16) -> u32 {
        let v = self.estimate_roc(seq);
        self.commit_roc(v, seq);
        v
    }
}
```

## Go envelope (signatures only)

```go
package srtp

type RecvRocTracker struct {
	roc         uint32
	sL          uint16
	initialized bool
}

func (t *RecvRocTracker) EstimateRoc(seq uint16, log ...zerolog.Logger) uint32
func (t *RecvRocTracker) CommitRoc(v uint32, seq uint16, log ...zerolog.Logger)
func (t *RecvRocTracker) GuessRoc(seq uint16, log ...zerolog.Logger) uint32
```

## Implementation suggestions (guidance, not authoritative)

- Port the signed half-range comparisons exactly.
- Keep estimation pure; only authenticated packets call `CommitRoc`.
- Preserve `GuessRoc` as the test/convenience composition of estimate plus commit.
