package protocol

import (
	"github.com/gorilla/websocket"
	"net/http"
	"sync"
)

// Sent by CLI when connecting
type RegisterMessage struct {
	Type     string `json:"type"`
	TunnelID string `json:"tunnelId"`
}

// Sent by Server to CLI
type TunnelRequest struct {
	ID      string              `json:"id"`
	Method  string              `json:"method"`
	Path    string              `json:"path"`
	Headers http.Header         `json:"headers"`
	Body    []byte              `json:"body"`
}

// Sent by CLI to Server
type TunnelResponse struct {
	ID      string              `json:"id"`
	Status  int                 `json:"status"`
	Headers http.Header         `json:"headers"`
	Body    []byte              `json:"body"`
}

type RegisterResponse struct {
	Type      string `json:"type"`
	PublicURL string `json:"publicUrl"`
}

type Tunnel struct {
	ID        string
	Conn      *websocket.Conn
	Send      chan interface{}
	Pending   map[string]chan TunnelResponse
	Mutex     sync.Mutex
	PublicURL string
}

