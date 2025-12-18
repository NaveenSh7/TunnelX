package main

import (
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

// -------- WebSocket (CLI) --------

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WS upgrade error:", err)
		return
	}

	var reg protocol.RegisterMessage
	if err := conn.ReadJSON(&reg); err != nil {
		log.Println("Register read error:", err)
		conn.Close()
		return
	}

	if reg.Type != "register" || reg.TunnelID == "" {
		conn.Close()
		return
	}

	// 1️⃣ Create Cloudflare public URL
	publicURL, err := createCloudflareURL(reg.TunnelID)
	if err != nil {
		conn.Close()
		return
	}

	// 2️⃣ Save tunnel mapping
	tunnel := &protocol.Tunnel{
		ID:        reg.TunnelID,
		Conn:      conn,
		PublicURL: publicURL,
	}

	mu.Lock()
	tunnels[reg.TunnelID] = tunnel
	mu.Unlock()

	log.Println("Tunnel registered:", reg.TunnelID)

	// 3️⃣ Send URL to CLI
	resp := protocol.RegisterResponse{
		Type:      "registered",
		PublicURL: publicURL,
	}
	conn.WriteJSON(resp)

	// 4️⃣ Keep WS alive
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			log.Println("Tunnel disconnected:", reg.TunnelID)
			mu.Lock()
			delete(tunnels, reg.TunnelID)
			mu.Unlock()
			return
		}
	}
}


func handleHTTP(w http.ResponseWriter, r *http.Request) {
	// URL: https://xyz.trycloudflare.com/t/{tunnelID}/path
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")

	if len(parts) < 2 || parts[0] != "t" {
		http.Error(w, "Invalid tunnel URL", 400)
		return
	}

	tunnelID := parts[1]
	forwardPath := "/"
	if len(parts) > 2 {
		forwardPath += strings.Join(parts[2:], "/")
	}

	mu.RLock()
	tunnel := tunnels[tunnelID]
	mu.RUnlock()

	if tunnel == nil {
		http.Error(w, "Tunnel not active", 503)
		return
	}

	body, _ := io.ReadAll(r.Body)

	req := protocol.TunnelRequest{
		Method:  r.Method,
		Path:    forwardPath,
		Headers: r.Header,
		Body:    body,
	}

	// 1️⃣ Forward request to CLI
	if err := tunnel.Conn.WriteJSON(req); err != nil {
		http.Error(w, "Tunnel write failed", 500)
		return
	}

	// 2️⃣ Read response from CLI
	var resp protocol.TunnelResponse
	if err := tunnel.Conn.ReadJSON(&resp); err != nil {
		http.Error(w, "Tunnel read failed", 500)
		return
	}

	// 3️⃣ Send response to browser
	for k, v := range resp.Headers {
		for _, val := range v {
			w.Header().Add(k, val)
		}
	}
	w.WriteHeader(resp.Status)
	w.Write(resp.Body)
}



func createCloudflareURL(tunnelID string) (string, error) {
	// Example: call cloudflared API / local process
	// For now mock:
	return "https://abcd-" + tunnelID[:6] + ".trycloudflare.com", nil
}

// helper to parse url from bando ka req
func getFullURL(r *http.Request) string {
	scheme := "http"

	// Behind Cloudflare / proxy
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	} else if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + r.URL.RequestURI()
}

func getPublicURL(r *http.Request, tunnelID string) string {
	scheme := "https"

	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		scheme = proto
	}

	return scheme + "://" + r.Host + "/t/" + tunnelID
}

