<!-- Datasheet = three things only: the reference source VERBATIM, the Go envelope
     (signatures, no bodies), and implementation suggestions. No behavioral summary,
     no implementation. The verbatim source is the only authoritative content. -->

# Datasheet: `meowcaller/session` authenticated receive

Media-pipeline receive composition for WARP authentication, ROC state, and SRTP
decryption.

**Validation vector:** `session_test.go` — valid peer round-trip, wrong-participant
rejection, changed-tag rejection, and valid receive after a rejected packet.

**Reference pinned at:** `2f001b5a3d6374cc5cf7177792c2a81f87a54080`

## Reference source (verbatim — authoritative excerpt)

```rust
    /// Inbound: verify the WARP MI tag, parse the header, decrypt the payload.
    /// The ROC is derived per-packet from the recv tracker (RFC 3711 guess-index), so the keystream
    /// stays aligned with the sender's across 16-bit seq wraps even under reorder/loss.
    ///
    /// The tag is authenticated (constant-time) against the *estimated* ROC BEFORE
    /// that ROC is committed, so an on-path relay can't fold unauthenticated packets
    /// into the rollover counter and permanently desync the receiver (RFC 3711
    /// §3.3.1 requires the index update to follow authentication).
    pub fn unprotect_audio(&mut self, packet: &[u8]) -> Option<(RtpHeader, Vec<u8>)> {
        if packet.len() < RTP_FIXED_HEADER_LEN + self.warp_mi_tag_len {
            return None;
        }
        let split = packet.len() - self.warp_mi_tag_len;
        let without_tag = &packet[..split];
        let received_tag = &packet[split..];
        let header = parse_rtp_header(without_tag)?;
        let header_len = rtp_header_byte_length(without_tag)?;
        if without_tag.len() <= header_len {
            return None;
        }
        let roc = self.recv_roc.estimate_roc(header.sequence_number);
        if !verify_warp_mi_tag(
            &self.recv_keys.auth_key,
            without_tag,
            roc,
            self.warp_mi_tag_len,
            received_tag,
        ) {
            return None;
        }
        // Authenticated: now it's safe to advance the rollover counter.
        self.recv_roc.commit_roc(roc, header.sequence_number);
        let cipher = &without_tag[header_len..];
        let plain = crypt_payload(
            &self.recv_keys,
            header.ssrc,
            header.sequence_number,
            roc,
            cipher,
        );
        Some((header, plain))
    }
```

## Go envelope (signatures only)

```go
package meowcaller

func (p *MediaPipeline) UnprotectAudio(packet []byte) (rtp.RtpHeader, []byte, bool)
```

## Implementation suggestions (guidance, not authoritative)

- Parse the RTP header before choosing the candidate ROC.
- Verify the configured-length WARP tag before committing ROC or decrypting.
- Hold the receive lock across estimate, verify, commit, and decrypt.
- Return `ok=false` without logging packet, key, tag, or plaintext bytes.
