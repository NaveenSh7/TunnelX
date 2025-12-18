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

const SERVER_WS = "ws://localhost:8080/ws"

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: tunnelx <port>")
		os.Exit(1)
	}

	port := os.Args[1]
	tunnelID := fmt.Sprintf("%d", time.Now().UnixNano())

	conn, _, err := websocket.DefaultDialer.Dial(SERVER_WS, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	conn.WriteJSON(protocol.RegisterMessage{
		Type:     "register",
		TunnelID: tunnelID,
	})

	var regResp protocol.RegisterResponse
	if err := conn.ReadJSON(&regResp); err != nil {
		log.Fatal(err)
	}

	fmt.Println("🌍 Public URL:", regResp.PublicURL)

	for {
		var req protocol.TunnelRequest
		if err := conn.ReadJSON(&req); err != nil {
			log.Fatal(err)
		}

		handleRequest(conn, req, port)
	}
}

func handleRequest(conn *websocket.Conn, treq protocol.TunnelRequest, port string) {
	url := "http://localhost:" + port + treq.Path

	httpReq, _ := http.NewRequest(treq.Method, url, bytes.NewReader(treq.Body))
	httpReq.Header = treq.Headers

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		sendError(conn)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	conn.WriteJSON(protocol.TunnelResponse{
	ID:      treq.ID,
	Status:  resp.StatusCode,
	Headers: resp.Header,
	Body:    body,
    })

}

func sendError(conn *websocket.Conn) {
	conn.WriteJSON(protocol.TunnelResponse{
		Status: 502,
		Headers: map[string][]string{
			"Content-Type": {"text/plain"},
		},
		Body: []byte("Bad Gateway"),
	})
}
