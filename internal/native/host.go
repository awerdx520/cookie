package native

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Host is the Native Messaging Host process.
// Chrome launches it and communicates via stdin/stdout.
// It also listens on a unix domain socket so cookie-cli can connect locally.
type Host struct {
	sockPath string
	listener net.Listener

	mu      sync.Mutex
	pending map[uint64]chan Response
	nextID  atomic.Uint64
}

// RunHost is the entry point for "cookie-cli native-messaging-host".
func RunHost() error {
	h := &Host{
		sockPath: SocketPath(),
		pending:  make(map[uint64]chan Response),
	}

	// Clean up stale socket file
	os.Remove(h.sockPath)

	ln, err := net.Listen("unix", h.sockPath)
	if err != nil {
		return fmt.Errorf("listen unix socket %s: %w", h.sockPath, err)
	}
	h.listener = ln
	defer h.cleanup()

	log.Printf("[native-host] 监听 socket: %s", h.sockPath)
	log.Printf("[native-host] 等待 Chrome 扩展通过 stdin/stdout 通信...")

	// Read responses from Chrome extension (stdin) in background
	go h.readFromExtension()

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		h.cleanup()
		os.Exit(0)
	}()

	// Accept CLI connections on the unix socket
	for {
		conn, err := ln.Accept()
		if err != nil {
			// Listener closed
			return nil
		}
		go h.handleCLIConn(conn)
	}
}

func (h *Host) cleanup() {
	if h.listener != nil {
		h.listener.Close()
	}
	os.Remove(h.sockPath)
}

// readFromExtension reads Native Messaging responses from stdin (Chrome)
// and dispatches them to pending requests.
func (h *Host) readFromExtension() {
	for {
		raw, err := ReadMessage(os.Stdin)
		if err != nil {
			log.Printf("[native-host] stdin 读取结束: %v", err)
			h.cleanup()
			os.Exit(0)
			return
		}

		var resp Response
		if err := json.Unmarshal(raw, &resp); err != nil {
			log.Printf("[native-host] 解析响应失败: %v", err)
			continue
		}

		h.mu.Lock()
		ch, ok := h.pending[resp.ID]
		h.mu.Unlock()

		if ok {
			ch <- resp
		}
	}
}

// sendToExtension writes a request to stdout (Chrome) and waits for the response.
func (h *Host) sendToExtension(req Request, timeout time.Duration) (*Response, error) {
	id := h.nextID.Add(1)
	req.ID = id

	ch := make(chan Response, 1)
	h.mu.Lock()
	h.pending[id] = ch
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		delete(h.pending, id)
		h.mu.Unlock()
	}()

	if err := WriteMessage(os.Stdout, req); err != nil {
		return nil, fmt.Errorf("write to extension: %w", err)
	}

	select {
	case resp := <-ch:
		return &resp, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("等待扩展响应超时 (%v)", timeout)
	}
}

// handleCLIConn handles one CLI connection on the unix socket.
// Protocol: the CLI sends a JSON Request, we forward it to Chrome, then write back the JSON Response.
func (h *Host) handleCLIConn(conn net.Conn) {
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(15 * time.Second))

	raw, err := ReadMessage(conn)
	if err != nil {
		log.Printf("[native-host] 读取 CLI 请求失败: %v", err)
		return
	}

	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		log.Printf("[native-host] 解析 CLI 请求失败: %v", err)
		return
	}

	// Handle export locally
	if req.Action == "exportCookies" {
		h.handleExport(conn, req)
		return
	}

	resp, err := h.sendToExtension(req, 10*time.Second)
	if err != nil {
		errResp := Response{ID: req.ID, OK: false, Error: err.Error()}
		WriteMessage(conn, errResp)
		return
	}

	WriteMessage(conn, resp)
}

// handleExport fetches all cookies for a domain and writes them to the export file.
func (h *Host) handleExport(conn net.Conn, req Request) {
	extReq := Request{Action: "getCookies", Domain: req.Domain}
	resp, err := h.sendToExtension(extReq, 10*time.Second)
	if err != nil {
		WriteMessage(conn, Response{ID: req.ID, OK: false, Error: err.Error()})
		return
	}
	if !resp.OK {
		WriteMessage(conn, Response{ID: req.ID, OK: false, Error: resp.Error})
		return
	}

	if err := WriteExport(req.Domain, resp.Data); err != nil {
		WriteMessage(conn, Response{ID: req.ID, OK: false, Error: err.Error()})
		return
	}

	WriteMessage(conn, Response{ID: req.ID, OK: true})
}

// SocketPath returns the path for the unix domain socket.
func SocketPath() string {
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		return dir + "/cookie-bridge.sock"
	}
	return "/tmp/cookie-bridge.sock"
}
