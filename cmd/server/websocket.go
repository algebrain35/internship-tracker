// Minimal RFC 6455 WebSocket server handshake.
// Uses only the Go standard library — no third-party packages.
// Demonstrates Go's net/http connection-hijacking mechanism.
//
// Code quality standards:
//   - Effective Go:            https://go.dev/doc/effective_go
//   - Go Code Review Comments: https://go.dev/wiki/CodeReviewComments
package main

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"

	"internship-tracker/internal/hub"
)

// wsGUID is the fixed magic string defined in RFC 6455 §1.3.
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// wsUpgrade performs the RFC 6455 WebSocket opening handshake.
// It hijacks the underlying TCP connection from the HTTP server
// and returns a hub.WSConn ready for frame exchange.
func wsUpgrade(w http.ResponseWriter, r *http.Request) (hub.WSConn, error) {
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "not a websocket request", http.StatusBadRequest)
		return nil, fmt.Errorf("missing Sec-WebSocket-Key")
	}

	// Accept key: base64(SHA-1(clientKey + GUID)) per RFC 6455 §4.2.2.
	h := sha1.New()
	io.WriteString(h, key+wsGUID)
	accept := base64.StdEncoding.EncodeToString(h.Sum(nil))

	// Hijack the HTTP connection so we can drive the TCP stream ourselves.
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, fmt.Errorf("server does not support connection hijacking")
	}
	conn, brw, err := hj.Hijack()
	if err != nil {
		return nil, fmt.Errorf("hijack: %w", err)
	}

	// Write the 101 Switching Protocols response into the buffered writer
	// and flush it — after this the connection is a WebSocket stream.
	_, err = fmt.Fprintf(brw,
		"HTTP/1.1 101 Switching Protocols\r\n"+
			"Upgrade: websocket\r\n"+
			"Connection: Upgrade\r\n"+
			"Sec-WebSocket-Accept: %s\r\n\r\n", accept)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("write handshake: %w", err)
	}
	if err := brw.Flush(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("flush handshake: %w", err)
	}

	return &wsConn{conn: conn, r: brw.Reader}, nil
}

// wsConn implements hub.WSConn over a hijacked TCP connection.
type wsConn struct {
	conn net.Conn
	r    *bufio.Reader
}

// WriteMessage sends a WebSocket frame.
// messageType follows the WebSocket opcode values: 1 = text, 2 = binary.
// Server-to-client frames are never masked (RFC 6455 §5.1).
func (c *wsConn) WriteMessage(messageType int, data []byte) error {
	n := len(data)

	// First byte: FIN bit (0x80) | opcode.
	hdr := []byte{byte(0x80 | messageType)}

	// Encode payload length per RFC 6455 §5.2.
	switch {
	case n <= 125:
		hdr = append(hdr, byte(n))
	case n <= 65535:
		hdr = append(hdr, 126, byte(n>>8), byte(n))
	default:
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(n))
		hdr = append(append(hdr, 127), b[:]...)
	}

	if _, err := c.conn.Write(hdr); err != nil {
		return err
	}
	_, err := c.conn.Write(data)
	return err
}

// ReadMessage reads the next WebSocket frame from the client.
// Client-to-server frames are always masked per RFC 6455 §5.3.
// Returns (opcode, payload, io.EOF) on a close frame or connection teardown.
func (c *wsConn) ReadMessage() (int, []byte, error) {
	var hdr [2]byte
	if _, err := io.ReadFull(c.r, hdr[:]); err != nil {
		return 0, nil, err
	}

	opcode := int(hdr[0] & 0x0f)
	masked := hdr[1]&0x80 != 0
	payloadLen := int64(hdr[1] & 0x7f)

	// Extended payload length (RFC 6455 §5.2).
	switch payloadLen {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(c.r, ext[:]); err != nil {
			return 0, nil, err
		}
		payloadLen = int64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(c.r, ext[:]); err != nil {
			return 0, nil, err
		}
		payloadLen = int64(binary.BigEndian.Uint64(ext[:]))
	}

	// Masking key (4 bytes, always present for client frames).
	var maskKey [4]byte
	if masked {
		if _, err := io.ReadFull(c.r, maskKey[:]); err != nil {
			return 0, nil, err
		}
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(c.r, payload); err != nil {
		return 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= maskKey[i%4]
		}
	}

	// Opcode 8 = connection close frame.
	if opcode == 8 {
		return opcode, payload, io.EOF
	}
	return opcode, payload, nil
}

// Close closes the underlying TCP connection.
func (c *wsConn) Close() error {
	return c.conn.Close()
}
