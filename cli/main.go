package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

const SERVER_WS = "wss://YOUR_AWS_SERVER/ws" // <-- change this

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

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: tunnel <port>")
		os.Exit(1)
	}

	port := os.Args[1]
	tunnelID := generateTunnelID()

	fmt.Println("🔗 Connecting to tunnel server...")
	conn, _, err := websocket.DefaultDialer.Dial(SERVER_WS, nil)
	if err != nil {
		log.Fatal("WebSocket connect failed:", err)
	}
	defer conn.Close()

	// Register tunnel
	reg := RegisterMessage{
		Type:     "register",
		TunnelID: tunnelID,
	}

	if err := conn.WriteJSON(reg); err != nil {
		log.Fatal("Register failed:", err)
	}

	fmt.Println("✅ Tunnel connected")
	fmt.Println("🌍 Public URL:")
	fmt.Printf("   https://<cloudflare-domain>/t/%s\n\n", tunnelID)
	fmt.Println("🚀 Forwarding to http://localhost:" + port)

	// Listen for incoming requests
	for {
		var req TunnelRequest
		if err := conn.ReadJSON(&req); err != nil {
			log.Fatal("Tunnel closed:", err)
		}

		go handleRequest(conn, req, port)
	}
}

func handleRequest(conn *websocket.Conn, treq TunnelRequest, port string) {
	url := "http://localhost:" + port + treq.Path

	httpReq, err := http.NewRequest(treq.Method, url, bytes.NewReader(treq.Body))
	if err != nil {
		sendError(conn)
		return
	}

	httpReq.Header = treq.Headers

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		sendError(conn)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	tresp := TunnelResponse{
		Status:  resp.StatusCode,
		Headers: resp.Header,
		Body:    body,
	}

	conn.WriteJSON(tresp)
}

func sendError(conn *websocket.Conn) {
	conn.WriteJSON(TunnelResponse{
		Status:  502,
		Headers: map[string][]string{"Content-Type": {"text/plain"}},
		Body:    []byte("Bad Gateway"),
	})
}

func generateTunnelID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
