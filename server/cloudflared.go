package main

import (
	"bufio"
	"log"
	"os/exec"
	"regexp"
	"sync/atomic"
	"time"
)

var publicURL atomic.Value // stores string

var urlRegex = regexp.MustCompile(`https://[a-zA-Z0-9-]+\.trycloudflare\.com`)

func startCloudflaredLoop() {
	go func() {
		for {
			log.Println("🚀 Starting cloudflared...")

			cmd := exec.Command(
				"cloudflared",
				"tunnel",
				"--url",
				"http://localhost:8080",
				"--no-autoupdate",
			)

			stdout, _ := cmd.StdoutPipe()
			stderr, _ := cmd.StderrPipe()

			if err := cmd.Start(); err != nil {
				log.Println("cloudflared start failed:", err)
				time.Sleep(5 * time.Second)
				continue
			}

			// Read logs concurrently
			go scanForURL(stdout)
			go scanForURL(stderr)

			// BLOCK until process exits
			err := cmd.Wait()
			log.Println("⚠️ cloudflared exited:", err)

			// Reset URL
			publicURL.Store("")

			// Backoff before restart
			time.Sleep(2 * time.Second)
		}
	}()
}

func scanForURL(pipe interface{}) {
	var scanner *bufio.Scanner

	switch p := pipe.(type) {
	case *bufio.Reader:
		scanner = bufio.NewScanner(p)
	default:
		scanner = bufio.NewScanner(pipe.(interface {
			Read([]byte) (int, error)
		}))
	}

	for scanner.Scan() {
		line := scanner.Text()
		log.Println("[cloudflared]", line)

		if url := urlRegex.FindString(line); url != "" {
			publicURL.Store(url)
			log.Println("🌍 Public URL:", url)
		}
	}
}
