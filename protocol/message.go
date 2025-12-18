package protocol

import (
	"github.com/gorilla/websocket"
)

// Sent by CLI when connecting
type RegisterMessage struct {
	Type     string `json:"type"`
	TunnelID string `json:"tunnelId"`
}

// Sent by Server to CLI
type TunnelRequest struct {
	Method  string              `json:"method"`
	Path    string              `json:"path"`
	Headers map[string][]string `json:"headers"`
	Body    []byte              `json:"body"`
}

// Sent by CLI to Server
type TunnelResponse struct {
	Status  int                 `json:"status"`
	Headers map[string][]string `json:"headers"`
	Body    []byte              `json:"body"`
}

type RegisterResponse struct {
	Type      string `json:"type"`
	PublicURL string `json:"publicUrl"`
}

type Tunnel struct {
	ID        string
	Conn      *websocket.Conn
	Send      chan interface{} // single writer channel
	PublicURL string
}
