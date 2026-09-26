// Package ws is a small RFC 6455 WebSocket implementation: a server
// upgrader for the browser terminal and a client dialer for Kubernetes
// exec/attach channels.
package ws

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const (
	OpContinuation = 0x0
	OpText         = 0x1
	OpBinary       = 0x2
	OpClose        = 0x8
	OpPing         = 0x9
	OpPong         = 0xA
)

const magic = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type Conn struct {
	conn     net.Conn
	br       *bufio.Reader
	client   bool // clients mask frames
	wmu      sync.Mutex
	Protocol string
	closed   bool
}

func accept(key string) string {
	h := sha1.New()
	h.Write([]byte(key + magic))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// Upgrade performs the server handshake. protocols lists acceptable
// subprotocols; the first one the client offers wins.
func Upgrade(w http.ResponseWriter, r *http.Request, protocols ...string) (*Conn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "expected websocket upgrade", http.StatusBadRequest)
		return nil, errors.New("not a websocket request")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "missing Sec-WebSocket-Key", http.StatusBadRequest)
		return nil, errors.New("missing key")
	}
	var proto string
	for _, offered := range strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",") {
		offered = strings.TrimSpace(offered)
		for _, p := range protocols {
			if offered == p && proto == "" {
				proto = p
			}
		}
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking unsupported", http.StatusInternalServerError)
		return nil, errors.New("hijack unsupported")
	}
	conn, brw, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	resp := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + accept(key) + "\r\n"
	if proto != "" {
		resp += "Sec-WebSocket-Protocol: " + proto + "\r\n"
	}
	resp += "\r\n"
	if _, err := conn.Write([]byte(resp)); err != nil {
		conn.Close()
		return nil, err
	}
	return &Conn{conn: conn, br: brw.Reader, Protocol: proto}, nil
}

// Dial opens a client connection. tlsConf and header carry credentials
// (bearer token, client certificate) for the Kubernetes API.
func Dial(rawURL string, header http.Header, tlsConf *tls.Config, protocols ...string) (*Conn, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	host := u.Host
	var conn net.Conn
	switch u.Scheme {
	case "wss", "https":
		if u.Port() == "" {
			host += ":443"
		}
		conf := tlsConf
		if conf == nil {
			conf = &tls.Config{}
		}
		conf = conf.Clone()
		if conf.ServerName == "" {
			conf.ServerName = u.Hostname()
		}
		conf.NextProtos = []string{"http/1.1"}
		conn, err = tls.Dial("tcp", host, conf)
	default:
		if u.Port() == "" {
			host += ":80"
		}
		conn, err = net.Dial("tcp", host)
	}
	if err != nil {
		return nil, err
	}
	kb := make([]byte, 16)
	rand.Read(kb)
	key := base64.StdEncoding.EncodeToString(kb)
	var b strings.Builder
	fmt.Fprintf(&b, "GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n", u.RequestURI(), u.Host, key)
	if len(protocols) > 0 {
		fmt.Fprintf(&b, "Sec-WebSocket-Protocol: %s\r\n", strings.Join(protocols, ", "))
	}
	for k, vs := range header {
		for _, v := range vs {
			fmt.Fprintf(&b, "%s: %s\r\n", k, v)
		}
	}
	b.WriteString("\r\n")
	if _, err := conn.Write([]byte(b.String())); err != nil {
		conn.Close()
		return nil, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: "GET"})
	if err != nil {
		conn.Close()
		return nil, err
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		conn.Close()
		return nil, fmt.Errorf("websocket handshake: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if resp.Header.Get("Sec-WebSocket-Accept") != accept(key) {
		conn.Close()
		return nil, errors.New("websocket handshake: bad accept key")
	}
	return &Conn{conn: conn, br: br, client: true, Protocol: resp.Header.Get("Sec-WebSocket-Protocol")}, nil
}

// WriteMessage sends one unfragmented frame.
func (c *Conn) WriteMessage(op byte, data []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.closed {
		return net.ErrClosed
	}
	hdr := make([]byte, 0, 14)
	hdr = append(hdr, 0x80|op)
	mask := byte(0)
	if c.client {
		mask = 0x80
	}
	n := len(data)
	switch {
	case n < 126:
		hdr = append(hdr, mask|byte(n))
	case n < 65536:
		hdr = append(hdr, mask|126, byte(n>>8), byte(n))
	default:
		hdr = append(hdr, mask|127)
		var l [8]byte
		binary.BigEndian.PutUint64(l[:], uint64(n))
		hdr = append(hdr, l[:]...)
	}
	payload := data
	if c.client {
		var mk [4]byte
		rand.Read(mk[:])
		hdr = append(hdr, mk[:]...)
		payload = make([]byte, n)
		for i := range data {
			payload[i] = data[i] ^ mk[i%4]
		}
	}
	if _, err := c.conn.Write(hdr); err != nil {
		return err
	}
	_, err := c.conn.Write(payload)
	return err
}

// ReadMessage returns the next complete data message, answering pings and
// reassembling fragments.
func (c *Conn) ReadMessage() (byte, []byte, error) {
	var msgOp byte
	var msg []byte
	for {
		var h [2]byte
		if _, err := io.ReadFull(c.br, h[:]); err != nil {
			return 0, nil, err
		}
		fin := h[0]&0x80 != 0
		op := h[0] & 0x0f
		masked := h[1]&0x80 != 0
		n := uint64(h[1] & 0x7f)
		switch n {
		case 126:
			var l [2]byte
			if _, err := io.ReadFull(c.br, l[:]); err != nil {
				return 0, nil, err
			}
			n = uint64(binary.BigEndian.Uint16(l[:]))
		case 127:
			var l [8]byte
			if _, err := io.ReadFull(c.br, l[:]); err != nil {
				return 0, nil, err
			}
			n = binary.BigEndian.Uint64(l[:])
		}
		if n > 64<<20 {
			return 0, nil, errors.New("websocket frame too large")
		}
		var mk [4]byte
		if masked {
			if _, err := io.ReadFull(c.br, mk[:]); err != nil {
				return 0, nil, err
			}
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(c.br, payload); err != nil {
			return 0, nil, err
		}
		if masked {
			for i := range payload {
				payload[i] ^= mk[i%4]
			}
		}
		switch op {
		case OpPing:
			c.WriteMessage(OpPong, payload)
			continue
		case OpPong:
			continue
		case OpClose:
			c.WriteMessage(OpClose, payload)
			c.Close()
			return OpClose, payload, io.EOF
		case OpContinuation:
			msg = append(msg, payload...)
		default:
			msgOp = op
			msg = payload
		}
		if fin {
			return msgOp, msg, nil
		}
	}
}

func (c *Conn) Close() error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.conn.Close()
}

// CloseWithMessage sends a close frame with a code and reason.
func (c *Conn) CloseWithMessage(code uint16, reason string) {
	b := make([]byte, 2+len(reason))
	binary.BigEndian.PutUint16(b, code)
	copy(b[2:], reason)
	c.WriteMessage(OpClose, b)
	c.Close()
}
