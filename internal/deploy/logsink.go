package deploy

import (
	"fmt"
	"os"
	"path/filepath"
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
