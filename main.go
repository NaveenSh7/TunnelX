package main

import (
	"log"
	"net/http"
)

func main() {
	http.HandleFunc("/ws", handleWebSocket) // CLI connects here
	http.HandleFunc("/", handleHTTP)        // Public traffic

	log.Println("TunnelX server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
