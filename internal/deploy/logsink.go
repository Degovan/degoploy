package deploy

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/degovan/degoploy/internal/realtime"
)

// maxLogSize caps one deployment's log; later output is dropped (the process keeps running).
const maxLogSize = 10 << 20

// logSink writes deployment output to its log file and streams each chunk with
// its byte offset, so clients can resume or de-duplicate precisely.
type logSink struct {
	path      string
	topic     string
	projectID int64
	hub       *realtime.Hub

	mu        sync.Mutex
	f         *os.File
	size      int64
	truncated bool
}

func newLogSink(dir string, deploymentID, projectID int64, hub *realtime.Hub) (*logSink, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, fmt.Sprintf("%d.log", deploymentID))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, err
	}
	return &logSink{
		path: path, topic: realtime.DeploymentLogTopic(deploymentID), projectID: projectID, hub: hub, f: f,
	}, nil
}

// Write never fails: losing log output must not break the deployment itself.
func (l *logSink) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.truncated || l.f == nil {
		return len(p), nil
	}
	chunk := p
	if room := maxLogSize - l.size; int64(len(chunk)) > room {
		chunk = append(chunk[:room:room], []byte(fmt.Sprintf("\n%s[degoploy] log exceeded %d MiB; further output is discarded%s\n",
			ansiYellow, maxLogSize>>20, ansiReset))...)
		l.truncated = true
	}
	n, _ := l.f.Write(chunk)
	if n > 0 {
		l.hub.Publish(realtime.Message{
			Topic: l.topic, Type: realtime.TypeLogChunk, ProjectID: l.projectID, Offset: l.size, Data: chunk[:n],
		})
		l.size += int64(n)
	}
	return len(p), nil
}

// Size is the number of bytes written so far.
func (l *logSink) Size() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.size
}

func (l *logSink) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return
	}
	l.f.Close()
	l.f = nil
	l.hub.Publish(realtime.Message{Topic: l.topic, Type: realtime.TypeLogEnd, ProjectID: l.projectID, Offset: l.size})
}

const (
	ansiReset  = "\x1b[0m"
	ansiBold   = "\x1b[1m"
	ansiDim    = "\x1b[2m"
	ansiRed    = "\x1b[1;31m"
	ansiGreen  = "\x1b[1;32m"
	ansiYellow = "\x1b[1;33m"
	ansiCyan   = "\x1b[1;36m"
)

// maxLineLen bounds the remembered line; longer lines are cut.
const maxLineLen = 300

// lastLine passes output through to w and remembers the last non-empty line
// (without ANSI escapes), to summarize why a command failed.
// Like exec's output copying it is used from one goroutine at a time.
type lastLine struct {
	w    io.Writer
	cur  []byte
	last string
}

func (l *lastLine) Write(p []byte) (int, error) {
	for _, b := range p {
		if b == '\n' || b == '\r' {
			l.endLine()
		} else if len(l.cur) < 4*maxLineLen {
			l.cur = append(l.cur, b)
		}
	}
	return l.w.Write(p)
}

func (l *lastLine) endLine() {
	line := strings.TrimSpace(ansiEscape.ReplaceAllString(string(l.cur), ""))
	if line != "" {
		l.last = truncate(strings.ToValidUTF8(line, "�"), maxLineLen)
	}
	l.cur = l.cur[:0]
}

// Last returns the last complete or partial line written.
func (l *lastLine) Last() string {
	l.endLine()
	return l.last
}

var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b[@-_]`)
