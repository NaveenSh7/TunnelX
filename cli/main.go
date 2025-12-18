package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"tunnelx/protocol"
)

const SERVER_WS = "ws://localhost:8080/ws" // change this

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

	// ---------- REGISTER ----------
	reg := protocol.RegisterMessage{
		Type:     "register",
		TunnelID: tunnelID,
	}

	if err := conn.WriteJSON(reg); err != nil {
		log.Fatal("Register failed:", err)
	}

	// ---------- READ PUBLIC URL ----------
	var resp protocol.RegisterResponse
	if err := conn.ReadJSON(&resp); err != nil {
		log.Fatal("Failed to read register response:", err)
	}

	if resp.Type != "registered" {
		log.Fatal("Invalid register response from server")
	}

	fmt.Println("✅ Tunnel connected")
	fmt.Println("🌍 Public URL:")
	fmt.Println("   " + resp.PublicURL)
	fmt.Println("🚀 Forwarding to http://localhost:" + port)
	fmt.Println()

	// ---------- TUNNEL LOOP ----------
	for {
		var req protocol.TunnelRequest
		if err := conn.ReadJSON(&req); err != nil {
			log.Fatal("Tunnel closed:", err)
		}

		go handleRequest(conn, req, port)
	}
}

func handleRequest(conn *websocket.Conn, treq protocol.TunnelRequest, port string) {
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

	tresp := protocol.TunnelResponse{
		Status:  resp.StatusCode,
		Headers: resp.Header,
		Body:    body,
	}

	conn.WriteJSON(tresp)
}

func sendError(conn *websocket.Conn) {
	conn.WriteJSON(protocol.TunnelResponse{
		Status:  502,
		Headers: map[string][]string{"Content-Type": {"text/plain"}},
		Body:    []byte("Bad Gateway"),
	})
}

func generateTunnelID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
