package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/degovan/degoploy/internal/auth"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/realtime"
	"github.com/degovan/degoploy/internal/store"
)

// wsRequest is a client → server message.
type wsRequest struct {
	Op     string `json:"op"` // subscribe | unsubscribe
	Topic  string `json:"topic"`
	Offset int64  `json:"offset"`
}

const (
	wsPingEvery      = 30 * time.Second
	wsWriteTimeout   = 10 * time.Second
	wsSessionRecheck = time.Minute
	logBackfillChunk = 64 << 10
)

// websocket serves /ws. Authentication is the session cookie; the Origin must be
// this host or base_url, so other sites cannot open a socket with the cookie.
func (a *API) websocket(w http.ResponseWriter, r *http.Request) {
	ident := auth.FromContext(r.Context())
	if ident == nil {
		httpx.Error(w, http.StatusUnauthorized, "unauthenticated", "login required")
		return
	}
	if ident.User.MustChangePassword {
		auth.PasswordChangeRequired(w)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: a.originPatterns()})
	if err != nil {
		return // Accept already wrote the error response
	}
	conn.SetReadLimit(4 << 10)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	// The user is re-read every minute so role changes reach open connections.
	var user atomic.Pointer[store.User]
	user.Store(ident.User)
	client := realtime.NewClient(func(projectID int64) bool {
		return a.authz.Can(context.Background(), user.Load(), auth.ActionProjectView, projectID)
	})
	a.hub.Register(client)
	defer a.hub.Unregister(client)

	go func() { // writer
		defer cancel()
		for b := range client.Outbox() {
			wctx, wcancel := context.WithTimeout(ctx, wsWriteTimeout)
			err := conn.Write(wctx, websocket.MessageText, b)
			wcancel()
			if err != nil {
				return
			}
		}
	}()

	go func() { // keepalive + session revalidation
		defer cancel()
		ping := time.NewTicker(wsPingEvery)
		recheck := time.NewTicker(wsSessionRecheck)
		defer ping.Stop()
		defer recheck.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ping.C:
				pctx, pcancel := context.WithTimeout(ctx, wsWriteTimeout)
				err := conn.Ping(pctx)
				pcancel()
				if err != nil {
					return
				}
			case <-recheck.C:
				_, u, err := a.store.SessionWithUser(ctx, ident.Session.IDHash)
				if err != nil || u.Disabled {
					conn.Close(websocket.StatusPolicyViolation, "session ended")
					return
				}
				user.Store(u)
			}
		}
	}()

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			break
		}
		var req wsRequest
		if json.Unmarshal(data, &req) != nil {
			continue
		}
		switch req.Op {
		case "subscribe":
			a.wsSubscribe(ctx, client, user.Load(), req)
		case "unsubscribe":
			a.hub.Unsubscribe(client, req.Topic)
		}
	}
	conn.Close(websocket.StatusNormalClosure, "")
}

func (a *API) wsSubscribe(ctx context.Context, c *realtime.Client, user *store.User, req wsRequest) {
	if req.Topic == realtime.TopicEvents {
		a.hub.Subscribe(c, req.Topic)
		return
	}
	id, ok := parseLogTopic(req.Topic)
	if !ok {
		a.hub.Send(c, realtime.Message{Topic: req.Topic, Type: realtime.TypeError, Data: "unknown topic"})
		return
	}
	d, err := a.store.DeploymentByID(ctx, id)
	if err != nil || !a.authz.Can(ctx, user, auth.ActionProjectView, d.ProjectID) {
		a.hub.Send(c, realtime.Message{Topic: req.Topic, Type: realtime.TypeError, Data: "deployment not found"})
		return
	}

	// Subscribe before reading the file so no chunk falls between backfill and
	// live output; clients drop bytes they already have using the offsets.
	a.hub.Subscribe(c, req.Topic)
	d, err = a.store.DeploymentByID(ctx, id)
	if err != nil {
		return
	}
	size := req.Offset
	if d.LogPath != "" {
		size, err = a.backfillLog(c, req.Topic, d.ProjectID, d.LogPath, req.Offset)
		if err != nil {
			slog.Warn("log backfill", "deployment", id, "err", err)
		}
	}
	if d.Status != store.DeployQueued && d.Status != store.DeployRunning {
		a.hub.Send(c, realtime.Message{Topic: req.Topic, Type: realtime.TypeLogEnd, ProjectID: d.ProjectID, Offset: size})
	}
}

// backfillLog sends the log file from offset to its current end and returns that end.
func (a *API) backfillLog(c *realtime.Client, topic string, projectID int64, path string, offset int64) (int64, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return offset, nil
	}
	if err != nil {
		return offset, err
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return offset, err
	}
	buf := make([]byte, logBackfillChunk)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			if !a.hub.Send(c, realtime.Message{Topic: topic, Type: realtime.TypeLogChunk, ProjectID: projectID, Offset: offset, Data: chunk}) {
				return offset, errors.New("client gone")
			}
			offset += int64(n)
		}
		if err == io.EOF {
			return offset, nil
		}
		if err != nil {
			return offset, err
		}
	}
}

func parseLogTopic(topic string) (int64, bool) {
	rest, ok := strings.CutPrefix(topic, "deployment:")
	if !ok {
		return 0, false
	}
	idStr, ok := strings.CutSuffix(rest, ":log")
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	return id, err == nil && id > 0
}

// originPatterns allows the configured public URL in addition to same-host origins.
func (a *API) originPatterns() []string {
	if a.cfg.BaseURL == "" {
		return nil
	}
	if u, err := url.Parse(a.cfg.BaseURL); err == nil && u.Host != "" {
		return []string{u.Host}
	}
	return nil
}
