package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// tunnelID → websocket connection
var tunnels = make(map[string]*websocket.Conn)
var mu sync.Mutex

// -------- WebSocket (CLI) --------

type RegisterMessage struct {
	Type     string `json:"type"`
	TunnelID string `json:"tunnelId"`
}

type TunnelRequest struct {
	Method  string              `json:"method"`
	Path    string              `json:"path"`
	Headers map[string][]string `json:"headers"`
	Body    []byte              `json:"body"`
}

type TunnelResponse struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers"`
	Body    []byte              `json:"body"`
}

func handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WS upgrade error:", err)
		return
	}

	// Read registration message
	_, msg, err := conn.ReadMessage()
	if err != nil {
		log.Println("WS read error:", err)
		return
	}

	var reg RegisterMessage
	if err := json.Unmarshal(msg, &reg); err != nil {
		log.Println("Invalid register message")
		return
	}

	if reg.Type != "register" || reg.TunnelID == "" {
		log.Println("Invalid registration data")
		return
	}

	// naya tunnel connection register karna
	mu.Lock()
	tunnels[reg.TunnelID] = conn
	mu.Unlock()

	log.Println("Tunnel registered:", reg.TunnelID)

	// Keep connection alive
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

// -------- HTTP (Public) --------

func handleHTTP(w http.ResponseWriter, r *http.Request) {
	// Expect: /t/{tunnelId}/path
	// random banda jab url per hit karega tabh
	fullURL := getFullURL(r)

	u, err := url.Parse(fullURL)
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	log.Println("parts", parts)
	if len(parts) < 2 || parts[0] != "t" {
		http.Error(w, "Invalid TunnelX URL", http.StatusBadRequest)
		return
	}

	tunnelID := parts[1]
	forwardPath := "/"
	if len(parts) == 4 {
		forwardPath += parts[3]
	}

	mu.Lock()
	conn := tunnels[tunnelID]
	mu.Unlock()

	if conn == nil {
		http.Error(w, "No active tunnel", http.StatusServiceUnavailable)
		return
	}

	// reqs details ko CLI ko send karna vai WS
	// Read request body
	body, _ := io.ReadAll(r.Body)

	req := TunnelRequest{
		Method:  r.Method,
		Path:    forwardPath,
		Headers: r.Header,
		Body:    body,
	}

	payload, _ := json.Marshal(req)

	err = conn.WriteMessage(websocket.TextMessage, payload)
	if err != nil {
		http.Error(w, "Tunnel write failed", 500)
		return
	}

	// CLI ka resp analyze karke wapis user ko bhejna
	// Read response from CLI
	_, msg, err := conn.ReadMessage()
	if err != nil {
		http.Error(w, "Tunnel read failed", 500)
		return
	}

	var resp TunnelResponse
	if err := json.Unmarshal(msg, &resp); err != nil {
		http.Error(w, "Invalid tunnel response", 500)
		return
	}

	// Write headers
	for k, v := range resp.Headers {
		for _, val := range v {
			w.Header().Add(k, val)
		}
	}

	w.WriteHeader(resp.Status)
	w.Write(resp.Body)

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
