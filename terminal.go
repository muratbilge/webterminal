package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// Basic auth already gates the endpoint; same-origin checks break when the
	// device is reached via IP, mDNS name, and hostname interchangeably.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Client->server messages are text frames with a one-byte prefix:
//
//	'0' + raw input bytes
//	'1' + JSON {"cols":N,"rows":N}   (resize)
//
// Server->client is raw PTY output in binary frames.
type resizeMsg struct {
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

func terminalHandler(shell string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("ws upgrade: %v", err)
			return
		}
		defer conn.Close()

		cmd := exec.Command(shell, "-l")
		cmd.Env = append(os.Environ(), "TERM=xterm-256color")
		ptmx, err := pty.Start(cmd)
		if err != nil {
			log.Printf("pty start %s: %v", shell, err)
			conn.WriteMessage(websocket.BinaryMessage, []byte("failed to start "+shell+": "+err.Error()+"\r\n"))
			return
		}
		log.Printf("session start: %s for %s", shell, r.RemoteAddr)

		done := make(chan struct{})

		// PTY -> websocket
		go func() {
			buf := make([]byte, 8192)
			for {
				n, err := ptmx.Read(buf)
				if n > 0 {
					conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
					if werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
						break
					}
				}
				if err != nil { // shell exited or PTY closed
					break
				}
			}
			conn.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shell exited"),
				time.Now().Add(time.Second))
			close(done)
		}()

		// websocket -> PTY
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				break
			}
			if len(data) == 0 {
				continue
			}
			switch data[0] {
			case '0':
				if _, err := ptmx.Write(data[1:]); err != nil {
					break
				}
			case '1':
				var rs resizeMsg
				if json.Unmarshal(data[1:], &rs) == nil && rs.Cols > 0 && rs.Rows > 0 {
					pty.Setsize(ptmx, &pty.Winsize{Cols: rs.Cols, Rows: rs.Rows})
				}
			}
		}

		// Client gone or shell exited: tear everything down.
		ptmx.Close()
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		cmd.Wait()
		<-done
		log.Printf("session end: %s", r.RemoteAddr)
	}
}
