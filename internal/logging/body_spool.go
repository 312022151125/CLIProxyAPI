package logging

import (
	"bufio"
	"bytes"
	"io"
	"os"

	log "github.com/sirupsen/logrus"
)

// defaultSpoolMemoryLimit is the payload size a spool keeps in memory before it
// spills to a temporary file. 1 MiB keeps short request/response bodies off the
// filesystem without meaningfully raising the process footprint.
const defaultSpoolMemoryLimit = 1 << 20

// bodySpool accumulates a payload in memory and spills to a temporary file in
// dir once the in-memory buffer would exceed limit. Each streaming request owns
// one spool; it is not safe for concurrent use.
type bodySpool struct {
	dir   string
	limit int
	mem   bytes.Buffer
	file  *os.File
	buf   *bufio.Writer
	path  string
}

// newBodySpool creates a spool that spills into dir after limit bytes.
// A limit of zero spills on the first non-empty write.
func newBodySpool(dir string, limit int) *bodySpool {
	return &bodySpool{dir: dir, limit: limit}
}

// Write appends p. The bytes are copied into the spool, so the caller keeps
// ownership of p and may reuse its buffer.
func (s *bodySpool) Write(p []byte) (int, error) {
	if s.file == nil && s.mem.Len()+len(p) <= s.limit {
		return s.mem.Write(p)
	}

	if s.file == nil {
		if errSpill := s.spill(); errSpill != nil {
			return 0, errSpill
		}
	}
	return s.buf.Write(p)
}

// spill moves the buffered bytes into a fresh temporary file in dir.
func (s *bodySpool) spill() error {
	file, errCreate := os.CreateTemp(s.dir, "spool-*.tmp")
	if errCreate != nil {
		return errCreate
	}
	// Buffer the spilled writes: a burst of small SSE chunks costs one write
	// syscall per 64 KiB instead of one per chunk.
	buffered := bufio.NewWriterSize(file, 64*1024)
	if _, errWrite := buffered.Write(s.mem.Bytes()); errWrite != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return errWrite
	}
	s.file = file
	s.buf = buffered
	s.path = file.Name()
	s.mem.Reset()
	return nil
}

// Bytes returns the payload when it never spilled, and nil once it did.
func (s *bodySpool) Bytes() []byte {
	if s.file != nil {
		return nil
	}
	return s.mem.Bytes()
}

// Path returns the temporary file path once the spool spilled, and "" otherwise.
// A spilled spool is flushed first, because the caller opens the path directly.
func (s *bodySpool) Path() string {
	if s.file != nil {
		if errFlush := s.buf.Flush(); errFlush != nil {
			log.WithError(errFlush).Warn("failed to flush body spool temp file")
		}
	}
	return s.path
}

// Reader returns a reader over the complete payload in write order. The caller
// closes the result. For a spilled spool this seeks the file back to offset 0.
func (s *bodySpool) Reader() (io.ReadCloser, error) {
	if s.file == nil {
		return io.NopCloser(bytes.NewReader(s.mem.Bytes())), nil
	}
	if errFlush := s.buf.Flush(); errFlush != nil {
		return nil, errFlush
	}
	if _, errSeek := s.file.Seek(0, io.SeekStart); errSeek != nil {
		return nil, errSeek
	}
	// The file itself is closed and removed by Cleanup, so the returned reader is
	// a non-closing view; Cleanup is the single owner of the descriptor.
	return io.NopCloser(s.file), nil
}

// Cleanup closes and removes the temporary file if one was created.
// Safe to call twice and safe when nothing spilled.
func (s *bodySpool) Cleanup() {
	if s.file != nil {
		if errFlush := s.buf.Flush(); errFlush != nil {
			log.WithError(errFlush).Warn("failed to flush body spool temp file")
		}
		if errClose := s.file.Close(); errClose != nil {
			log.WithError(errClose).Warn("failed to close body spool temp file")
		}
		s.file = nil
		s.buf = nil
	}
	if s.path != "" {
		if errRemove := os.Remove(s.path); errRemove != nil {
			log.WithError(errRemove).Warn("failed to remove body spool temp file")
		}
		s.path = ""
	}
}
