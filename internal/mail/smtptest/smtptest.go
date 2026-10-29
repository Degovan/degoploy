// Package smtptest is a minimal plain-text SMTP server for tests.
package smtptest

import (
	"bufio"
	"encoding/base64"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// Message is one mail the server accepted.
type Message struct {
	From     string
	To       []string
	Data     string
	Username string
	Password string
}

type Server struct {
	Host string
	Port int
	// RejectRcpt makes RCPT TO answer 550, like an unknown mailbox.
	RejectRcpt bool

	ln       net.Listener
	mu       sync.Mutex
	messages []Message
}

// New starts a server on a free loopback port and stops it when t ends.
func New(t testing.TB) *Server {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port, ln: ln}
	go s.serve()
	t.Cleanup(func() { ln.Close() })
	return s
}

func (s *Server) Messages() []Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Message{}, s.messages...)
}

func (s *Server) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	reply := func(line string) { conn.Write([]byte(line + "\r\n")) }
	reply("220 smtptest ready")
	var m Message
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		cmd := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			reply("250-smtptest")
			reply("250 AUTH PLAIN")
		case strings.HasPrefix(cmd, "AUTH PLAIN"):
			raw, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(line[len("AUTH PLAIN"):]))
			parts := strings.Split(string(raw), "\x00")
			if len(parts) == 3 {
				m.Username, m.Password = parts[1], parts[2]
			}
			reply("235 2.7.0 Authentication successful")
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			m.From = strings.Trim(line[len("MAIL FROM:"):], "<> ")
			reply("250 2.1.0 OK")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			if s.RejectRcpt {
				reply("550 5.1.1 No such user")
				continue
			}
			m.To = append(m.To, strings.Trim(line[len("RCPT TO:"):], "<> "))
			reply("250 2.1.5 OK")
		case cmd == "DATA":
			reply("354 Go ahead")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				b.WriteString(strings.TrimPrefix(l, "."))
			}
			m.Data = b.String()
			s.mu.Lock()
			s.messages = append(s.messages, m)
			n := len(s.messages)
			s.mu.Unlock()
			reply("250 2.0.0 OK queued as " + strconv.Itoa(n))
			m = Message{Username: m.Username, Password: m.Password}
		case cmd == "QUIT":
			reply("221 Bye")
			return
		case cmd == "RSET", cmd == "NOOP":
			reply("250 OK")
		default:
			reply("502 Command not implemented")
		}
	}
}
