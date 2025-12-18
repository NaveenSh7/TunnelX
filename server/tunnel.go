package main

import (
	//"encoding/json"
	//"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"tunnelx/protocol"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

var (
	tunnels = make(map[string]*protocol.Tunnel)
	mu      sync.RWMutex
)

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

	if reg.Type != "register" || reg.TunnelID == "" {
		conn.Close()
		return
	}

	base := getPublicURL()
	if base == "" {
		conn.Close()
		return
	}

	tunnel := &protocol.Tunnel{
		ID:        reg.TunnelID,
		Conn:      conn,
		Send:      make(chan interface{}, 8),
		PublicURL: base + "/share/" + reg.TunnelID,
	}

	mu.Lock()
	tunnels[reg.TunnelID] = tunnel
	mu.Unlock()

	// 🔑 single writer goroutine
	go func() {
		for msg := range tunnel.Send {
			if err := conn.WriteJSON(msg); err != nil {
				return
			}
		}
	}()

	// Send URL to CLI
	tunnel.Send <- protocol.RegisterResponse{
		Type:      "registered",
		PublicURL: tunnel.PublicURL,
	}

	log.Println("Tunnel registered:", reg.TunnelID)

	// single reader loop
	for {
		var resp protocol.TunnelResponse
		if err := conn.ReadJSON(&resp); err != nil {
			mu.Lock()
			delete(tunnels, reg.TunnelID)
			mu.Unlock()
			return
		}
	}
}

func handleHTTP(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")

	if len(parts) < 2 || parts[0] != "share" {
		http.Error(w, "Invalid TunnelX URL", 400)
		return
	}

	tunnelID := parts[1]
	path := "/"
	if len(parts) > 2 {
		path += strings.Join(parts[2:], "/")
	}

	mu.RLock()
	tunnel := tunnels[tunnelID]
	mu.RUnlock()

	if tunnel == nil {
		http.Error(w, "Tunnel inactive", 503)
		return
	}

	body, _ := io.ReadAll(r.Body)

	req := protocol.TunnelRequest{
		Method:  r.Method,
		Path:    path,
		Headers: r.Header,
		Body:    body,
	}

	tunnel.Send <- req

	var resp protocol.TunnelResponse
	if err := tunnel.Conn.ReadJSON(&resp); err != nil {
		http.Error(w, "Tunnel read failed", 500)
		return
	}

	for k, v := range resp.Headers {
		for _, val := range v {
			w.Header().Add(k, val)
		}
	}
	w.WriteHeader(resp.Status)
	w.Write(resp.Body)
}

// helper to grab Global url from cloudflared.go
func getPublicURL() string {
	val := publicURL.Load()
	if val == nil {
		return ""
	}
	return val.(string)
}
