// Command mlowtest encodes raw PCM to an MLow .bin and decodes an MLow .bin back to
// audio, so you can record from a mic and listen to the reconstruction for quality.
//
// MLow operates on 16 kHz mono audio in 60 ms (960-sample) frames. The .bin container
// is trivial: the 4-byte magic "MLW1" followed by, per frame, a little-endian uint16
// byte length and that many MLow frame bytes (frames are variable length).
//
//	mlowtest encode [-i in.raw] [-o out.bin]   # raw s16le mono 16k (stdin) -> .bin
//	mlowtest decode [-i in.bin] [-o out.wav]   # .bin -> WAV 16k mono (or -raw to stdout)
//
// Pair it with scripts/mlow_mic_test.sh to drive the mic and playback.
package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/purpshell/meowcaller/mlow"
)

const (
	sampleRate   = 16000
	frameSamples = 960 // 60 ms @ 16 kHz
	magic        = "MLW1"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	var err error
	switch os.Args[1] {
	case "encode":
		err = encode(os.Args[2:])
	case "decode":
		err = decode(os.Args[2:])
	default:
		usage()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "mlowtest:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  mlowtest encode [-i in.raw] [-o out.bin]   raw s16le mono 16kHz (default stdin) -> MLow .bin
  mlowtest decode [-i in.bin] [-o out.wav]   MLow .bin -> WAV 16kHz mono (-raw: s16le to stdout)`)
	os.Exit(2)
}

// parseFlags is a tiny -k v / -flag parser (avoids the global flag pkg quirks with subcommands).
func parseFlags(args []string) (in, out string, raw bool, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-i":
			if i+1 >= len(args) {
				return "", "", false, errors.New("-i needs a path")
			}
			i++
			in = args[i]
		case "-o":
			if i+1 >= len(args) {
				return "", "", false, errors.New("-o needs a path")
			}
			i++
			out = args[i]
		case "-raw":
			raw = true
		default:
			return "", "", false, fmt.Errorf("unknown arg %q", args[i])
		}
	}
	return in, out, raw, nil
}

func openIn(path string) (io.ReadCloser, error) {
	if path == "" || path == "-" {
		return io.NopCloser(os.Stdin), nil
	}
	return os.Open(path)
}

func openOut(path string) (io.WriteCloser, error) {
	if path == "" || path == "-" {
		return nopWriteCloser{os.Stdout}, nil
	}
	return os.Create(path)
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

func encode(args []string) error {
	in, out, _, err := parseFlags(args)
	if err != nil {
		return err
	}
	if out == "" {
		out = "out.bin"
	}
	rc, err := openIn(in)
	if err != nil {
		return err
	}
	defer rc.Close()
	wc, err := openOut(out)
	if err != nil {
		return err
	}
	defer wc.Close()

	r := bufio.NewReader(rc)
	bw := bufio.NewWriter(wc)
	defer bw.Flush()
	if _, err := bw.WriteString(magic); err != nil {
		return err
	}

	enc := mlow.NewMlowEncoder()
	pcm := make([]float32, frameSamples)
	buf := make([]byte, frameSamples*2)
	var frames, samples int
	for {
		n, rerr := io.ReadFull(r, buf)
		if n == 0 {
			if rerr == io.EOF {
				break
			}
			return rerr
		}
		// Zero-pad a trailing partial frame.
		for i := n; i < len(buf); i++ {
			buf[i] = 0
		}
		for i := 0; i < frameSamples; i++ {
			pcm[i] = float32(int16(binary.LittleEndian.Uint16(buf[2*i:]))) / 32768.0
		}
		frame, eerr := enc.Encode(pcm)
		if eerr != nil {
			return eerr
		}
		if len(frame) > 0xffff {
			return fmt.Errorf("frame too large: %d bytes", len(frame))
		}
		var lp [2]byte
		binary.LittleEndian.PutUint16(lp[:], uint16(len(frame)))
		if _, err := bw.Write(lp[:]); err != nil {
			return err
		}
		if _, err := bw.Write(frame); err != nil {
			return err
		}
		frames++
		samples += frameSamples
		if rerr == io.ErrUnexpectedEOF {
			break
		}
	}
	fmt.Fprintf(os.Stderr, "encoded %d frames (%.2fs) -> %s\n", frames, float64(samples)/sampleRate, out)
	return nil
}

func decode(args []string) error {
	in, out, raw, err := parseFlags(args)
	if err != nil {
		return err
	}
	if out == "" && !raw {
		out = "out.wav"
	}
	rc, err := openIn(in)
	if err != nil {
		return err
	}
	defer rc.Close()
	r := bufio.NewReader(rc)

	hdr := make([]byte, 4)
	if _, err := io.ReadFull(r, hdr); err != nil {
		return fmt.Errorf("read magic: %w", err)
	}
	if string(hdr) != magic {
		return fmt.Errorf("bad magic %q (not an MLow .bin)", hdr)
	}

	dec := mlow.NewMlowDecoder()
	var pcm []int16
	for {
		var lp [2]byte
		if _, err := io.ReadFull(r, lp[:]); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		ln := int(binary.LittleEndian.Uint16(lp[:]))
		frame := make([]byte, ln)
		if _, err := io.ReadFull(r, frame); err != nil {
			return fmt.Errorf("short frame: %w", err)
		}
		for _, v := range dec.Decode(frame) {
			s := v * 32768.0
			if s > 32767 {
				s = 32767
			}
			if s < -32768 {
				s = -32768
			}
			pcm = append(pcm, int16(s))
		}
	}

	wc, err := openOut(out)
	if err != nil {
		return err
	}
	defer wc.Close()
	bw := bufio.NewWriter(wc)
	defer bw.Flush()

	if raw {
		b := make([]byte, 2)
		for _, s := range pcm {
			binary.LittleEndian.PutUint16(b, uint16(s))
			bw.Write(b)
		}
		fmt.Fprintf(os.Stderr, "decoded %d samples (%.2fs) -> raw s16le\n", len(pcm), float64(len(pcm))/sampleRate)
		return nil
	}
	if err := writeWAV(bw, pcm); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "decoded %d samples (%.2fs) -> %s\n", len(pcm), float64(len(pcm))/sampleRate, out)
	return nil
}

// writeWAV writes a canonical 16-bit mono PCM WAV (16 kHz).
func writeWAV(w io.Writer, pcm []int16) error {
	dataLen := len(pcm) * 2
	var h [44]byte
	copy(h[0:], "RIFF")
	binary.LittleEndian.PutUint32(h[4:], uint32(36+dataLen))
	copy(h[8:], "WAVE")
	copy(h[12:], "fmt ")
	binary.LittleEndian.PutUint32(h[16:], 16) // PCM fmt chunk size
	binary.LittleEndian.PutUint16(h[20:], 1)  // PCM
	binary.LittleEndian.PutUint16(h[22:], 1)  // mono
	binary.LittleEndian.PutUint32(h[24:], sampleRate)
	binary.LittleEndian.PutUint32(h[28:], sampleRate*2) // byte rate
	binary.LittleEndian.PutUint16(h[32:], 2)            // block align
	binary.LittleEndian.PutUint16(h[34:], 16)           // bits/sample
	copy(h[36:], "data")
	binary.LittleEndian.PutUint32(h[40:], uint32(dataLen))
	if _, err := w.Write(h[:]); err != nil {
		return err
	}
	b := make([]byte, 2)
	for _, s := range pcm {
		binary.LittleEndian.PutUint16(b, uint16(s))
		if _, err := w.Write(b); err != nil {
			return err
		}
	}
	return nil
}
