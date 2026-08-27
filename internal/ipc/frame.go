// Package ipc is the wire format both ends of Engramux's named pipe speak —
// the frame codec, the JSON envelope it carries, the ingest ACK contract,
// and the derivation of the pipe name they meet on (spec 5.2, 5.3). It says
// nothing about how the pipe itself is listened on or dialed; that belongs
// to the pipe-server and relay tasks built on top of this package.
package ipc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// MaxFrameLen is the largest payload ReadFrame accepts, checked against the
// 4-byte length header before any allocation happens. 4 MiB is 8x the 512
// KiB per-field value cap (spec 6): a single envelope payload can
// legitimately carry several fields each near that cap plus JSON structural
// overhead, so a cap at or below 512 KiB would make some legitimate
// payloads permanently unsendable rather than merely rare.
const MaxFrameLen = 4 * 1024 * 1024

// ErrFrameTooLarge is returned by ReadFrame when the 4-byte length header
// names more than MaxFrameLen, and by WriteFrame when payload already
// exceeds it. ReadFrame returns it before allocating a buffer for the body.
var ErrFrameTooLarge = errors.New("ipc: frame length exceeds cap")

// WriteFrame writes payload as a 4-byte little-endian length followed by
// payload itself (spec 5.2). It writes nothing if payload exceeds
// MaxFrameLen.
func WriteFrame(w io.Writer, payload []byte) error {
	if len(payload) > MaxFrameLen {
		return fmt.Errorf("%w: %d bytes, cap %d", ErrFrameTooLarge, len(payload), MaxFrameLen)
	}

	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], uint32(len(payload))) //nolint:gosec // G115: len(payload) <= MaxFrameLen (4 MiB), checked above
	if _, err := w.Write(header[:]); err != nil {
		return fmt.Errorf("ipc: write frame length: %w", err)
	}
	if _, err := w.Write(payload); err != nil {
		return fmt.Errorf("ipc: write frame payload: %w", err)
	}
	return nil
}

// ReadFrame reads one frame: a 4-byte little-endian length, then that many
// bytes. The length is checked against MaxFrameLen before any allocation,
// so a 4-byte header from an untrusted writer cannot name a gigabyte and
// have ReadFrame allocate it. A short read at either stage — including a
// body cut off mid-frame — is surfaced as an error via io.ReadFull, never
// silently treated as a complete, shorter frame.
func ReadFrame(r io.Reader) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, fmt.Errorf("ipc: read frame length: %w", err)
	}

	n := binary.LittleEndian.Uint32(header[:])
	if n > MaxFrameLen {
		return nil, fmt.Errorf("%w: %d bytes, cap %d", ErrFrameTooLarge, n, MaxFrameLen)
	}

	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("ipc: read frame payload: %w", err)
	}
	return payload, nil
}
