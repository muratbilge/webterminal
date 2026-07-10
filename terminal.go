package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

// killSession SIGKILLs every process whose session ID is sid. pty.Start makes
// the shell a session leader, so this reliably reaps background jobs the
// shell left behind — bash forwards SIGHUP to its jobs, but not on every exit
// path, and other shells differ.
func killSession(sid int) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		stat, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		// Field 6 (1-based) is the session ID; fields 1-2 are "pid (comm)"
		// where comm may contain spaces, so parse from after the last ')'.
		s := string(stat)
		i := strings.LastIndexByte(s, ')')
		if i < 0 {
			continue
		}
		fields := strings.Fields(s[i+1:])
		if len(fields) < 4 {
			continue
		}
		if fsid, err := strconv.Atoi(fields[3]); err == nil && fsid == sid {
			syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}

const (
	pingInterval = 30 * time.Second
	// A connection that hasn't answered a ping within this window is dead;
	// without it, clients that vanish mid-air (wifi drop, no TCP FIN) would
	// leak the shell and PTY forever.
	pongTimeout = 75 * time.Second
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	// Same-host origins only. Basic auth cannot stop cross-site websocket
	// hijacking: the browser replays cached credentials on the upgrade no
	// matter which page opened the socket, so the Origin must be checked.
	// Origin-less requests (non-browser clients) are allowed — they manage
	// their own credentials.
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		u, err := url.Parse(origin)
		return err == nil && u.Host == r.Host
	},
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

		conn.SetReadDeadline(time.Now().Add(pongTimeout))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(pongTimeout))
			return nil
		})
		stopPing := make(chan struct{})
		defer close(stopPing)
		go func() {
			t := time.NewTicker(pingInterval)
			defer t.Stop()
			for {
				select {
				case <-t.C:
					conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second))
				case <-stopPing:
					return
				}
			}
		}()

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
	readLoop:
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
					break readLoop
				}
			case '1':
				var rs resizeMsg
				if json.Unmarshal(data[1:], &rs) == nil && rs.Cols > 0 && rs.Rows > 0 {
					pty.Setsize(ptmx, &pty.Winsize{Cols: rs.Cols, Rows: rs.Rows})
				}
			}
		}

		// Client gone or shell exited: tear everything down. SIGHUP must go
		// out *before* the PTY closes — on EOF bash exits normally without
		// HUPing its jobs (huponexit is off by default), orphaning background
		// processes; on SIGHUP it resends the signal to every job first.
		if cmd.Process != nil {
			cmd.Process.Signal(syscall.SIGHUP)
		}
		ptmx.Close()
		waited := make(chan struct{})
		go func() {
			cmd.Wait()
			close(waited)
		}()
		select {
		case <-waited:
		case <-time.After(3 * time.Second):
			if cmd.Process != nil {
				cmd.Process.Kill()
			}
			<-waited
		}
		killSession(cmd.Process.Pid)
		<-done
		log.Printf("session end: %s", r.RemoteAddr)
	}
}
