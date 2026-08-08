<!-- Datasheet = three things only: the reference source VERBATIM, the Go envelope
     (signatures, no bodies), and implementation suggestions. No behavioral summary,
     no implementation. The verbatim source is the only authoritative content. -->

# Datasheet: `srtp/warp`

WARP RTP extension constants and the WARP MESSAGE-INTEGRITY tag.

**Validation vector:** `srtp/testdata/kats.json` — `e2e_srtp.peer_authKey`,
`inputs.samplePacket`, `inputs.roc`, and `e2e_srtp.warp_mi_tag4`. Receive
verification additionally rejects a changed tag, participant key, ROC, and invalid
tag length.

**Reference pinned at:** `2f001b5a3d6374cc5cf7177792c2a81f87a54080`

## Reference source (verbatim — authoritative)

```rust
//! WARP RTP extension constants and the WARP MESSAGE-INTEGRITY tag.
//!
//! wacrg spec: warp-crypto (CRY-07), warp relay framing (REL-03). The MI tag is keyed
//! by the per-participant SRTP auth key (KAT-pinned), NOT a separate callKey-derived
//! "warp auth key" as the spec's `derive_warp_auth_key` documents; don't "fix" it
//! toward the spec without re-checking the vectors.

use hmac::{Hmac, KeyInit, Mac};
use sha1::Sha1;
use subtle::ConstantTimeEq;

type HmacSha1 = Hmac<Sha1>;

pub const WARP_AUDIO_PIGGYBACK_EXT: [u8; 4] = [0x30, 0x01, 0x00, 0x00];
pub const WARP_MI_TAG_LEN: usize = 4;
/// Packets #1-2 carry an empty extension; #3+ (0-based index >= 2) piggyback.
pub const WARP_PIGGYBACK_START_PACKET: usize = 2;

/// Audio piggyback extension word for `packet_index`, or `None` for the first packets.
pub fn audio_piggyback_extension_for(
    packet_index: usize,
    enabled: bool,
    start_packet: usize,
) -> Option<u32> {
    if !enabled || packet_index < start_packet {
        return None;
    }
    Some(u32::from_be_bytes(WARP_AUDIO_PIGGYBACK_EXT))
}

/// WARP MI tag = first `tag_len` bytes of HMAC-SHA1(auth_key, packet || roc_be32).
pub fn compute_warp_mi_tag(
    auth_key: &[u8],
    packet_without_tag: &[u8],
    roc: u32,
    tag_len: usize,
) -> Vec<u8> {
    let mut mac = HmacSha1::new_from_slice(auth_key).expect("HMAC accepts any key length");
    mac.update(packet_without_tag);
    mac.update(&roc.to_be_bytes());
    let full = mac.finalize().into_bytes();
    full[..tag_len].to_vec()
}

/// Constant-time-verify a received WARP MI tag against the one we compute for
/// `roc`. Callers must reject a packet whose tag fails BEFORE folding recv ROC
/// state, so an unauthenticated packet can't desync the rollover counter
/// (RFC 3711 §3.3.1: update the index only after authentication).
pub fn verify_warp_mi_tag(
    auth_key: &[u8],
    packet_without_tag: &[u8],
    roc: u32,
    tag_len: usize,
    received_tag: &[u8],
) -> bool {
    let expected = compute_warp_mi_tag(auth_key, packet_without_tag, roc, tag_len);
    expected.ct_eq(received_tag).into()
}

/// Append the WARP MI tag to a protected packet.
pub fn append_warp_mi_tag(
    auth_key: &[u8],
    packet_without_tag: &[u8],
    roc: u32,
    tag_len: usize,
) -> Vec<u8> {
    let tag = compute_warp_mi_tag(auth_key, packet_without_tag, roc, tag_len);
    let mut out = Vec::with_capacity(packet_without_tag.len() + tag.len());
    out.extend_from_slice(packet_without_tag);
    out.extend_from_slice(&tag);
    out
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::voip::testkat::{hexd, kats};

    #[test]
    fn warp_mi_tag_matches_kat() {
        let k = kats();
        let auth_key = hexd(&k, &["e2e_srtp", "peer_authKey"]);
        let packet = hexd(&k, &["inputs", "samplePacket"]);
        let roc = k["inputs"]["roc"].as_u64().unwrap() as u32;
        let tag = compute_warp_mi_tag(&auth_key, &packet, roc, WARP_MI_TAG_LEN);
        assert_eq!(
            hex::encode(tag),
            k["e2e_srtp"]["warp_mi_tag4"].as_str().unwrap()
        );
    }
}
```

## Go envelope (signatures only)

```go
package srtp

const (
	WarpExtProfile          uint16 = 0xdebe
	WarpMITagLen                   = 4
	WarpPiggybackStartPacket       = 2
)

var WarpAudioPiggybackExt = [4]byte{0x30, 0x01, 0x00, 0x00}

func AudioPiggybackExtensionFor(packetIndex int, enabled bool, startPacket int, log ...zerolog.Logger) *uint32
func ComputeWarpMITag(authKey, packetWithoutTag []byte, roc uint32, tagLen int, log ...zerolog.Logger) []byte
func AppendWarpMITag(authKey, packetWithoutTag []byte, roc uint32, tagLen int, log ...zerolog.Logger) []byte
func VerifyWarpMITag(authKey, packetWithoutTag []byte, roc uint32, tagLen int, receivedTag []byte, log ...zerolog.Logger) bool
```

## Implementation suggestions (guidance, not authoritative)

- Recompute the received tag with the reference formula and the received tag
  length, then compare with `hmac.Equal`.
- Reject zero-length and greater-than-SHA-1-length tags before slicing the computed
  digest.
- Do not log the authentication key, protected packet, or tag.
