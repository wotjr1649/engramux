package ipc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

// TestWriteFrame_Bytes pins the wire bytes WriteFrame produces: a 4-byte
// little-endian length, then the payload, and nothing else.
func TestWriteFrame_Bytes(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteFrame(&buf, []byte("hi")); err != nil {
		t.Fatalf("WriteFrame: %v", err)
	}
	want := []byte{0x02, 0x00, 0x00, 0x00, 'h', 'i'}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Errorf("WriteFrame wrote %x, want %x", buf.Bytes(), want)
	}
}

// TestWriteFrame_RejectsOversizedPayload asserts the writer will not put an
// unreadable frame on the wire: a payload already over MaxFrameLen is
// rejected before anything is written.
func TestWriteFrame_RejectsOversizedPayload(t *testing.T) {
	payload := make([]byte, MaxFrameLen+1)
	var buf bytes.Buffer
	err := WriteFrame(&buf, payload)
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("WriteFrame error = %v, want errors.Is(_, ErrFrameTooLarge)", err)
	}
	if buf.Len() != 0 {
		t.Errorf("WriteFrame wrote %d bytes despite rejecting the payload", buf.Len())
	}
}

// TestFrameRoundTrip covers the sizes that matter: empty, small, and exactly
// at the cap.
func TestFrameRoundTrip(t *testing.T) {
	cases := map[string][]byte{
		"empty":  {},
		"small":  []byte(`{"a":1}`),
		"at cap": bytes.Repeat([]byte("x"), MaxFrameLen),
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteFrame(&buf, payload); err != nil {
				t.Fatalf("WriteFrame: %v", err)
			}
			got, err := ReadFrame(&buf)
			if err != nil {
				t.Fatalf("ReadFrame: %v", err)
			}
			if !bytes.Equal(got, payload) {
				t.Errorf("ReadFrame round-trip: got %d bytes, want %d bytes", len(got), len(payload))
			}
		})
	}
}

// TestReadFrame_RejectsOversizedLength is the allocation-safety gate: a
// header naming a gigabyte must be rejected before ReadFrame ever attempts
// to allocate or read a body that size. The reader below supplies only the
// 4-byte header; if ReadFrame reads (or allocates) the declared body before
// checking the cap, it either hangs reading past EOF and returns the wrong
// error, or it actually performs a huge allocation. Either way the
// errors.Is assertion below fails.
func TestReadFrame_RejectsOversizedLength(t *testing.T) {
	const gigabyte = 1 << 30 // spec language: "must not be allowed to name a gigabyte"
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], gigabyte)

	_, err := ReadFrame(bytes.NewReader(header[:]))
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("ReadFrame error = %v, want errors.Is(_, ErrFrameTooLarge)", err)
	}
}

// TestReadFrame_RejectsLengthOneOverCap pins the boundary precisely: the cap
// itself is fine (see TestFrameRoundTrip's "at cap" case), one byte over it
// is not.
func TestReadFrame_RejectsLengthOneOverCap(t *testing.T) {
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], MaxFrameLen+1)

	_, err := ReadFrame(bytes.NewReader(header[:]))
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Fatalf("ReadFrame error = %v, want errors.Is(_, ErrFrameTooLarge)", err)
	}
}

// TestReadFrame_TruncatedHeader: the stream ends before the 4-byte length
// even arrives.
func TestReadFrame_TruncatedHeader(t *testing.T) {
	_, err := ReadFrame(bytes.NewReader([]byte{0x01, 0x02}))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("ReadFrame error = %v, want errors.Is(_, io.ErrUnexpectedEOF)", err)
	}
}

// TestReadFrame_TruncatedBody: the header is intact and names 10 bytes, but
// the stream closes after 5. A partial read must not be treated as success.
func TestReadFrame_TruncatedBody(t *testing.T) {
	var header [4]byte
	binary.LittleEndian.PutUint32(header[:], 10)
	wire := append(header[:], []byte("short")...) // 5 of the promised 10 bytes

	got, err := ReadFrame(bytes.NewReader(wire))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("ReadFrame error = %v, want errors.Is(_, io.ErrUnexpectedEOF)", err)
	}
	if got != nil {
		t.Errorf("ReadFrame returned %d bytes on a truncated frame, want nil", len(got))
	}
}
