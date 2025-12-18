package main

import (
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"tunnelx/protocol"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

var (
	tunnels = make(map[string]*Tunnel)
	mu      sync.RWMutex
)

type Tunnel struct {
	ID      string
	Conn    *websocket.Conn
	Send    chan interface{}
	Pending map[string]chan protocol.TunnelResponse
	Mutex   sync.Mutex
}

// ===================== WEBSOCKET =====================

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	var reg protocol.RegisterMessage
	if err := conn.ReadJSON(&reg); err != nil {
		conn.Close()
		return
	}

	base := getPublicURL()
	if reg.Type != "register" || reg.TunnelID == "" || base == "" {
		conn.Close()
		return
	}

	tunnel := &Tunnel{
		ID:      reg.TunnelID,
		Conn:    conn,
		Send:    make(chan interface{}, 32),
		Pending: make(map[string]chan protocol.TunnelResponse),
	}

	mu.Lock()
	tunnels[reg.TunnelID] = tunnel
	mu.Unlock()

	go wsWriter(tunnel)
	go wsReader(tunnel)

	tunnel.Send <- protocol.RegisterResponse{
		Type:      "registered",
		PublicURL: base + "/share/" + reg.TunnelID,
	}

	log.Println("✅ Tunnel registered:", reg.TunnelID)
}

func wsReader(t *Tunnel) {
	defer cleanupTunnel(t)

	for {
		var resp protocol.TunnelResponse
		if err := t.Conn.ReadJSON(&resp); err != nil {
			return
		}
		log.Println("📥 WS RESPONSE")
log.Println("ID:", resp.ID)
log.Println("Status:", resp.Status)
log.Println("Headers:", resp.Headers)
log.Println("Body size:", len(resp.Body))

		t.Mutex.Lock()
		ch := t.Pending[resp.ID]
		delete(t.Pending, resp.ID)
		t.Mutex.Unlock()

		if ch != nil {
			ch <- resp
		}
	}
}

func wsWriter(t *Tunnel) {

	for msg := range t.Send {

		if err := t.Conn.WriteJSON(msg); err != nil {
			return
		}
	}
}

// ===================== HTTP PROXY =====================

func handleHTTP(w http.ResponseWriter, r *http.Request) {
	log.Println("➡️  HTTP IN")
log.Println("Method:", r.Method)
log.Println("URL:", r.URL.Path)
log.Println("Headers:")
for k, v := range r.Header {
	log.Println(" ", k, ":", v)
}
	path := strings.TrimPrefix(r.URL.Path, "/share/")
	parts := strings.SplitN(path, "/", 2)

	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	tunnelID := parts[0]
	forwardPath := "/"
	if len(parts) == 2 {
		forwardPath += parts[1]
	}

	mu.RLock()
	tunnel := tunnels[tunnelID]
	mu.RUnlock()

	log.Println("🎯 Tunnel found:", tunnelID)
log.Println("➡️ Forward path:", forwardPath)


	if tunnel == nil {
		http.NotFound(w, r)
		return
	}

	body, _ := io.ReadAll(r.Body)
	reqID := generateID()

	respCh := make(chan protocol.TunnelResponse, 1)
	tunnel.Mutex.Lock()
	tunnel.Pending[reqID] = respCh
	tunnel.Mutex.Unlock()

	tunnel.Send <- protocol.TunnelRequest{
		ID:      reqID,
		Method:  r.Method,
		Path:    forwardPath,
		Headers: r.Header,
		Body:    body,
	}

	select {
	case resp := <-respCh:
		log.Println("⬅️ Response from CLI")
        log.Println("Status:", resp.Status)
        log.Println("Headers:")
        for k, v := range resp.Headers {
        	log.Println(" ", k, ":", v)
        }
        log.Println("Body size:", len(resp.Body))

		copyHeaders(w, resp.Headers)
		w.WriteHeader(resp.Status)
		w.Write(resp.Body)

	case <-time.After(30 * time.Second):
		http.Error(w, "Gateway Timeout", 504)
	}
}

// ===================== HEADER FIX =====================

func copyHeaders(w http.ResponseWriter, h http.Header) {
	// Remove default headers
	for k := range w.Header() {
		w.Header().Del(k)
	}

	for k, v := range h {
		if isHopByHop(k) {
			continue
		}
		for _, val := range v {
			w.Header().Add(k, val)
		}
	}
}

func isHopByHop(h string) bool {
	switch strings.ToLower(h) {
	case "connection",
		"keep-alive",
		"proxy-authenticate",
		"proxy-authorization",
		"te",
		"trailer",
		"transfer-encoding",
		"upgrade",
		"content-length":
		return true
	default:
		return false
	}
}

// ===================== CLEANUP =====================

func cleanupTunnel(t *Tunnel) {
	mu.Lock()
	delete(tunnels, t.ID)
	mu.Unlock()

	close(t.Send)
	t.Conn.Close()
	log.Println("❌ Tunnel closed:", t.ID)
}

// ===================== UTIL =====================

func generateID() string {
	return time.Now().Format("20060102150405.000000000")
}

func getPublicURL() string {
	val := publicURL.Load()
	if val == nil {
		return ""
	}
	return val.(string)
}
