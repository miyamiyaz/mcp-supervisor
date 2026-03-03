package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Transport handles newline-delimited JSON-RPC over a reader/writer pair.
type Transport struct {
	scanner *bufio.Scanner
	writer  io.Writer
	mu      sync.Mutex // protects writer
}

func NewTransport(r io.Reader, w io.Writer) *Transport {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // up to 10MB per line
	return &Transport{scanner: s, writer: w}
}

// ReadMessage reads one JSON-RPC message. Returns io.EOF at end of input.
func (t *Transport) ReadMessage() (json.RawMessage, error) {
	if !t.scanner.Scan() {
		if err := t.scanner.Err(); err != nil {
			return nil, fmt.Errorf("read: %w", err)
		}
		return nil, io.EOF
	}
	raw := t.scanner.Bytes()
	msg := make(json.RawMessage, len(raw))
	copy(msg, raw)
	return msg, nil
}

// WriteMessage writes one JSON-RPC message followed by a newline.
func (t *Transport) WriteMessage(msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, err := t.writer.Write(data); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if _, err := t.writer.Write([]byte("\n")); err != nil {
		return fmt.Errorf("write newline: %w", err)
	}
	return nil
}
