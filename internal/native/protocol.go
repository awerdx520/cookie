// Package native implements Chrome Native Messaging protocol and
// a local unix domain socket bridge so that cookie-cli can query
// Chrome cookies without a long-running serve process.
package native

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const maxMessageSize = 1024 * 1024 // 1 MiB, Chrome's limit

// ReadMessage reads one Native Messaging frame from r:
// 4-byte little-endian length prefix followed by UTF-8 JSON.
func ReadMessage(r io.Reader) (json.RawMessage, error) {
	var length uint32
	if err := binary.Read(r, binary.LittleEndian, &length); err != nil {
		return nil, fmt.Errorf("read length prefix: %w", err)
	}
	if length > maxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes (max %d)", length, maxMessageSize)
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("read message body: %w", err)
	}
	return json.RawMessage(buf), nil
}

// WriteMessage writes one Native Messaging frame to w.
func WriteMessage(w io.Writer, msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	if len(data) > maxMessageSize {
		return fmt.Errorf("message too large: %d bytes (max %d)", len(data), maxMessageSize)
	}
	length := uint32(len(data))
	if err := binary.Write(w, binary.LittleEndian, length); err != nil {
		return fmt.Errorf("write length prefix: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write message body: %w", err)
	}
	return nil
}

// Request is sent from the native host (or CLI via socket) to Chrome extension.
type Request struct {
	ID     uint64 `json:"id"`
	Action string `json:"action"`
	Domain string `json:"domain,omitempty"`
	URL    string `json:"url,omitempty"`
	Name   string `json:"name,omitempty"`
}

// Response is returned by the Chrome extension.
type Response struct {
	ID    uint64          `json:"id"`
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}
