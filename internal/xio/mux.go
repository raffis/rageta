package xio

import (
	"encoding/binary"
	"errors"
	"io"
	"sync"

	"golang.org/x/sync/errgroup"
)

// muxReadBufSize bounds a single frame's payload; larger reads are simply
// split across multiple frames.
const muxReadBufSize = 32 * 1024

type MuxSource struct {
	ID byte
	R  io.Reader
}

// Mux reads from each source concurrently and writes its data to w as a
// stream of frames, each prefixed with a 5-byte header (1-byte source ID,
// 4-byte big-endian payload length). Writes to w are serialized so a frame's
// header and payload are never interleaved with another source's frame.
func Mux(w io.Writer, r ...MuxSource) error {
	var wg errgroup.Group
	var mu sync.Mutex

	writeFrame := func(id byte, buf []byte) error {
		mu.Lock()
		defer mu.Unlock()

		var header [5]byte
		header[0] = id
		binary.BigEndian.PutUint32(header[1:], uint32(len(buf)))

		if _, err := w.Write(header[:]); err != nil {
			return err
		}
		if len(buf) > 0 {
			if _, err := w.Write(buf); err != nil {
				return err
			}
		}
		return nil
	}

	for _, src := range r {
		wg.Go(func() error {
			buf := make([]byte, muxReadBufSize)
			for {
				n, err := src.R.Read(buf)
				isEOF := errors.Is(err, io.EOF)
				if err != nil && !isEOF {
					return err
				}

				if n > 0 {
					if err := writeFrame(src.ID, buf[:n]); err != nil {
						return err
					}
				}

				if isEOF {
					return nil
				}
			}
		})
	}

	return wg.Wait()
}

// NewDemuxer creates a Demuxer with no writers registered. Register a
// writer per stream ID with WithWriter before writing any muxed data into
// it — frames whose ID has no registered writer are dropped.
func NewDemuxer() *Demuxer {
	return &Demuxer{w: make(map[byte]io.Writer)}
}

// Demuxer is an io.Writer that accepts a stream of frames produced by Mux
// and, for each frame, forwards its payload to the io.Writer registered for
// that frame's ID.
type Demuxer struct {
	w   map[byte]io.Writer
	buf []byte
}

func (d *Demuxer) WithSink(id byte, w io.Writer) *Demuxer {
	d.w[id] = w
	return d
}

func (d *Demuxer) Write(p []byte) (int, error) {
	d.buf = append(d.buf, p...)

	for {
		if len(d.buf) < 5 {
			break
		}

		n := binary.BigEndian.Uint32(d.buf[1:5])
		if uint32(len(d.buf)-5) < n {
			break
		}

		id := d.buf[0]
		payload := d.buf[5 : 5+n]
		d.buf = d.buf[5+n:]

		w, ok := d.w[id]
		if !ok || w == nil {
			continue
		}

		if _, err := w.Write(payload); err != nil {
			return 0, err
		}
	}

	return len(p), nil
}
