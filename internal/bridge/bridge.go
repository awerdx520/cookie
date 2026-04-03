// Package bridge 实现 Chrome 扩展 WebSocket 桥接服务。
//
// 架构:
//
//	HTTP 客户端 ──GET /cookies?domain=x──▶ Go HTTP Server ──ws──▶ Chrome Extension
//	                                      (127.0.0.1:8008)        (chrome.cookies API)
//
// Chrome 扩展通过 WebSocket 长连接到 Go 服务；
// Go 服务收到 HTTP 请求后，通过 WebSocket 发给扩展，等待扩展返回结果，再写回 HTTP 响应。
package bridge

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// request 是发给 Chrome 扩展的请求
type request struct {
	ID     uint64 `json:"id"`
	Action string `json:"action"`
	Domain string `json:"domain,omitempty"`
	URL    string `json:"url,omitempty"`
	Name   string `json:"name,omitempty"`
}

// response 是 Chrome 扩展返回的响应
type response struct {
	ID    uint64          `json:"id"`
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// Server 是桥接服务
type Server struct {
	addr     string
	upgrader websocket.Upgrader

	mu       sync.Mutex
	conn     *websocket.Conn
	pending  map[uint64]chan response
	nextID   atomic.Uint64
}

// NewServer 创建桥接服务，addr 格式如 "127.0.0.1:8008"
func NewServer(addr string) *Server {
	s := &Server{
		addr:    addr,
		pending: make(map[uint64]chan response),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
	return s
}

// ListenAndServe 启动 HTTP + WebSocket 服务
func (s *Server) ListenAndServe() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWS)
	mux.HandleFunc("/cookies", s.handleGetCookies)
	mux.HandleFunc("/domains", s.handleListDomains)
	mux.HandleFunc("/health", s.handleHealth)

	log.Printf("Cookie Bridge 服务启动: http://%s", s.addr)
	log.Printf("  GET /cookies?domain=example.com  获取 Cookie")
	log.Printf("  GET /domains                     列出所有域名")
	log.Printf("  GET /health                      健康检查")
	log.Printf("等待 Chrome 扩展连接...")

	return http.ListenAndServe(s.addr, mux)
}

// sendToExtension 发送请求给 Chrome 扩展并等待响应
func (s *Server) sendToExtension(req request, timeout time.Duration) (*response, error) {
	s.mu.Lock()
	conn := s.conn
	if conn == nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("Chrome 扩展未连接，请确保 Chrome 已启动且 Cookie Bridge 扩展已安装")
	}

	id := s.nextID.Add(1)
	req.ID = id
	ch := make(chan response, 1)
	s.pending[id] = ch
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
	}()

	if err := conn.WriteJSON(req); err != nil {
		return nil, fmt.Errorf("发送请求到扩展失败: %w", err)
	}

	select {
	case resp := <-ch:
		return &resp, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("等待扩展响应超时 (%v)", timeout)
	}
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade 失败: %v", err)
		return
	}

	s.mu.Lock()
	old := s.conn
	s.conn = conn
	s.mu.Unlock()

	if old != nil {
		old.Close()
	}

	log.Printf("Chrome 扩展已连接 (%s)", r.RemoteAddr)

	defer func() {
		s.mu.Lock()
		if s.conn == conn {
			s.conn = nil
		}
		s.mu.Unlock()
		conn.Close()
		log.Printf("Chrome 扩展已断开")
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("WebSocket 读取错误: %v", err)
			}
			return
		}

		var resp response
		if err := json.Unmarshal(msg, &resp); err != nil {
			log.Printf("解析扩展响应失败: %v", err)
			continue
		}

		s.mu.Lock()
		ch, ok := s.pending[resp.ID]
		s.mu.Unlock()

		if ok {
			ch <- resp
		}
	}
}

func (s *Server) handleGetCookies(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	name := r.URL.Query().Get("name")
	url := r.URL.Query().Get("url")

	if domain == "" && url == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "需要 domain 或 url 参数"})
		return
	}

	req := request{
		Action: "getCookies",
		Domain: domain,
		URL:    url,
		Name:   name,
	}

	resp, err := s.sendToExtension(req, 10*time.Second)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}

	if !resp.OK {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": resp.Error})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	// resp.Data 已经是 JSON 数组，直接写入
	fmt.Fprintf(w, `{"ok":true,"cookies":%s}`, resp.Data)
}

func (s *Server) handleListDomains(w http.ResponseWriter, r *http.Request) {
	req := request{Action: "listDomains"}

	resp, err := s.sendToExtension(req, 10*time.Second)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}

	if !resp.OK {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": resp.Error})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"ok":true,"domains":%s}`, resp.Data)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	connected := s.conn != nil
	s.mu.Unlock()

	status := map[string]interface{}{
		"service":   "cookie-bridge",
		"extension": connected,
	}
	writeJSON(w, http.StatusOK, status)
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
