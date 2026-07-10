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
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

const (
	pingInterval = 30 * time.Second
	// A connection that hasn't answered a ping within this window is dead;
	// without it, clients that vanish mid-air (wifi drop, no TCP FIN) would
	// hold the attachment forever.
	pongTimeout = 75 * time.Second
	// How much recent output is kept per session to replay on re-attach.
	replayMax = 128 * 1024
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

// A session is a running shell in a PTY that outlives any single websocket:
// the browser identifies it by a token, so a page refresh or short network
// drop re-attaches to the same shell instead of starting a new one. A
// detached session is killed after the grace period.
type session struct {
	id    string
	ptmx  *os.File
	cmd   *exec.Cmd
	grace time.Duration

	mu     sync.Mutex
	conn   *websocket.Conn // attached client, nil while detached
	buf    []byte          // recent output for replay on re-attach
	kill   *time.Timer     // fires terminate() after grace when detached
	exited bool
}

var (
	sessions   = map[string]*session{}
	sessionsMu sync.Mutex
)

func getOrCreateSession(id, shell string, grace time.Duration) (*session, bool, error) {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	if s, ok := sessions[id]; ok {
		return s, true, nil
	}
	cmd := exec.Command(shell, "-l")
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, false, err
	}
	s := &session{id: id, ptmx: ptmx, cmd: cmd, grace: grace}
	sessions[id] = s
	go s.readLoop()
	return s, false, nil
}

// readLoop pumps PTY output into the replay buffer and the attached client
// for the session's whole life, then runs the full cleanup when the shell
// exits (or terminate closes the PTY). All websocket data writes happen under
// s.mu — gorilla/websocket allows only one concurrent writer.
func (s *session) readLoop() {
	buf := make([]byte, 8192)
	for {
		n, err := s.ptmx.Read(buf)
		if n > 0 {
			s.mu.Lock()
			s.buf = append(s.buf, buf[:n]...)
			if len(s.buf) > replayMax {
				s.buf = append([]byte(nil), s.buf[len(s.buf)-replayMax:]...)
			}
			if s.conn != nil {
				s.conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
				if werr := s.conn.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
					s.conn.Close()
					s.conn = nil
					s.startKillTimerLocked()
				}
			}
			s.mu.Unlock()
		}
		if err != nil { // shell exited or PTY closed by terminate()
			break
		}
	}

	s.mu.Lock()
	s.exited = true
	c := s.conn
	s.conn = nil
	if s.kill != nil {
		s.kill.Stop()
	}
	s.mu.Unlock()

	sessionsMu.Lock()
	delete(sessions, s.id)
	sessionsMu.Unlock()

	if c != nil {
		c.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shell exited"),
			time.Now().Add(time.Second))
		c.Close()
	}

	// SIGHUP before closing the PTY fd so the shell (if still alive) can
	// forward it to its jobs; escalate, then sweep the whole terminal
	// session so background jobs can't outlive it.
	if s.cmd.Process != nil {
		s.cmd.Process.Signal(syscall.SIGHUP)
	}
	waited := make(chan struct{})
	go func() {
		s.cmd.Wait()
		close(waited)
	}()
	select {
	case <-waited:
	case <-time.After(3 * time.Second):
		if s.cmd.Process != nil {
			s.cmd.Process.Kill()
		}
		<-waited
	}
	killSession(s.cmd.Process.Pid)
	s.ptmx.Close()
	log.Printf("session %.8s… ended", s.id)
}

// terminate ends a session that overstayed its detach grace period.
func (s *session) terminate() {
	s.mu.Lock()
	if s.exited {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	log.Printf("session %.8s… grace expired, terminating", s.id)
	if s.cmd.Process != nil {
		s.cmd.Process.Signal(syscall.SIGHUP)
	}
	s.ptmx.Close() // unblocks readLoop, which runs the full cleanup
}

func (s *session) startKillTimerLocked() {
	if s.kill != nil {
		s.kill.Stop()
	}
	s.kill = time.AfterFunc(s.grace, s.terminate)
}

// attach makes conn the session's client, displacing any previous one, and
// replays recent output so the terminal picks up where it left off.
func (s *session) attach(conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.kill != nil {
		s.kill.Stop()
		s.kill = nil
	}
	if s.conn != nil {
		s.conn.WriteControl(websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseGoingAway, "attached elsewhere"),
			time.Now().Add(time.Second))
		s.conn.Close()
	}
	s.conn = conn
	if len(s.buf) > 0 {
		conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
		conn.WriteMessage(websocket.BinaryMessage, s.buf)
	}
}

// detach disconnects conn if it is still the attached client and starts the
// grace countdown. Safe to call with a conn that was already displaced.
func (s *session) detach(conn *websocket.Conn) {
	s.mu.Lock()
	if s.conn == conn {
		s.conn = nil
		if !s.exited {
			s.startKillTimerLocked()
		}
	}
	s.mu.Unlock()
	conn.Close()
}

func validToken(t string) bool {
	if len(t) < 8 || len(t) > 64 {
		return false
	}
	for _, c := range t {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func terminalHandler(shell string, grace time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("session")
		if !validToken(token) {
			http.Error(w, "missing or malformed session token", http.StatusBadRequest)
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("ws upgrade: %v", err)
			return
		}

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

		s, resumed, err := getOrCreateSession(token, shell, grace)
		if err != nil {
			log.Printf("pty start %s: %v", shell, err)
			conn.WriteMessage(websocket.BinaryMessage, []byte("failed to start "+shell+": "+err.Error()+"\r\n"))
			conn.Close()
			return
		}
		s.attach(conn)
		verb := "started"
		if resumed {
			verb = "resumed"
		}
		log.Printf("session %.8s… %s for %s", token, verb, r.RemoteAddr)

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
				if _, err := s.ptmx.Write(data[1:]); err != nil {
					break readLoop
				}
			case '1':
				var rs resizeMsg
				if json.Unmarshal(data[1:], &rs) == nil && rs.Cols > 0 && rs.Rows > 0 {
					pty.Setsize(s.ptmx, &pty.Winsize{Cols: rs.Cols, Rows: rs.Rows})
				}
			}
		}

		s.detach(conn)
		log.Printf("session %.8s… detached from %s", token, r.RemoteAddr)
	}
}

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
