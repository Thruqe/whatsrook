package commands

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestWebPMetadataLossy(t *testing.T) {
	// Create a minimal WebP header and VP8 chunk (lossy)
	vp8Payload := []byte{
		0x00, 0x00, 0x00, // frame tag
		0x9d, 0x01, 0x2a, // keyframe start code
		0x00, 0x02,       // width 512 (little endian)
		0x00, 0x02,       // height 512 (little endian)
	}

	var mockWebP bytes.Buffer
	mockWebP.WriteString("RIFF")
	// Size placeholder
	mockWebP.Write([]byte{0, 0, 0, 0})
	mockWebP.WriteString("WEBP")

	mockWebP.WriteString("VP8 ")
	var sizeBuf [4]byte
	binary.LittleEndian.PutUint32(sizeBuf[:], uint32(len(vp8Payload)))
	mockWebP.Write(sizeBuf[:])
	mockWebP.Write(vp8Payload)

	data := mockWebP.Bytes()
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))

	// Test injecting exif
	pack := "My Test Pack"
	author := "My Test Author"
	output, err := AddStickerMetadata(data, pack, author)
	if err != nil {
		t.Fatalf("failed to add sticker metadata: %v", err)
	}

	// Test extracting exif
	meta, err := GetStickerMetadata(output)
	if err != nil {
		t.Fatalf("failed to get sticker metadata: %v", err)
	}
	if meta == nil {
		t.Fatal("sticker metadata is nil")
	}
	if meta.PackName != pack {
		t.Errorf("expected pack name %q, got %q", pack, meta.PackName)
	}
	if meta.Publisher != author {
		t.Errorf("expected publisher %q, got %q", author, meta.Publisher)
	}
	if meta.PackID == "" {
		t.Error("expected pack id to be generated, got empty")
	}
}

func TestWebPMetadataLossless(t *testing.T) {
	// Create a minimal WebP header and VP8L chunk (lossless)
	// VP8L signature is 0x2f
	// Width 512, Height 512, alpha = 0:
	// w-1 = 511 (0x01ff), h-1 = 511 (0x01ff), alpha = 0
	// bit layout for uint32 starting at byte 1:
	// bits 0..13: 511 (0x01ff)
	// bits 14..27: 511 (0x01ff)
	// bit 28: 0 (no alpha)
	// val = 511 | (511 << 14) = 511 | 8372224 = 8372735 = 0x7FC1FF
	vp8lPayload := []byte{
		0x2f,             // signature
		0xff, 0xc1, 0x7f, 0x00, // val (0x7FC1FF)
	}

	var mockWebP bytes.Buffer
	mockWebP.WriteString("RIFF")
	mockWebP.Write([]byte{0, 0, 0, 0})
	mockWebP.WriteString("WEBP")

	mockWebP.WriteString("VP8L")
	var sizeBuf [4]byte
	binary.LittleEndian.PutUint32(sizeBuf[:], uint32(len(vp8lPayload)))
	mockWebP.Write(sizeBuf[:])
	mockWebP.Write(vp8lPayload)

	data := mockWebP.Bytes()
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))

	pack := "Lossless Pack"
	author := "Lossless Author"
	output, err := AddStickerMetadata(data, pack, author)
	if err != nil {
		t.Fatalf("failed to add sticker metadata: %v", err)
	}

	meta, err := GetStickerMetadata(output)
	if err != nil {
		t.Fatalf("failed to get sticker metadata: %v", err)
	}
	if meta == nil {
		t.Fatal("sticker metadata is nil")
	}
	if meta.PackName != pack {
		t.Errorf("expected pack name %q, got %q", pack, meta.PackName)
	}
	if meta.Publisher != author {
		t.Errorf("expected publisher %q, got %q", author, meta.Publisher)
	}
}
