package api_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/degovan/degoploy/internal/api"
	"github.com/degovan/degoploy/internal/store"
)

type wsMsg struct {
	Topic     string          `json:"topic"`
	Type      string          `json:"type"`
	ProjectID int64           `json:"project_id"`
	Offset    int64           `json:"offset"`
	Data      json.RawMessage `json:"data"`
}

func dialWS(t *testing.T, base string, c *client) *websocket.Conn {
	t.Helper()
	hdr := http.Header{}
	for _, ck := range c.http.Jar.Cookies(mustURL(base)) {
		hdr.Add("Cookie", ck.Name+"="+ck.Value)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(base, "http")+"/ws", &websocket.DialOptions{HTTPHeader: hdr})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.CloseNow() })
	return conn
}

func send(t *testing.T, conn *websocket.Conn, v string) {
	t.Helper()
	if err := conn.Write(context.Background(), websocket.MessageText, []byte(v)); err != nil {
		t.Fatal(err)
	}
}

// readUntil reads messages until pred matches or the timeout passes.
func readUntil(t *testing.T, conn *websocket.Conn, pred func(wsMsg) bool) []wsMsg {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var seen []wsMsg
	for {
		_, b, err := conn.Read(ctx)
		if err != nil {
			t.Fatalf("waiting for message: %v (seen %d)", err, len(seen))
		}
		var m wsMsg
		json.Unmarshal(b, &m)
		seen = append(seen, m)
		if pred(m) {
			return seen
		}
	}
}

func TestWebSocketRequiresLogin(t *testing.T) {
	srv, _ := newTestServer(t)
	_, resp, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err == nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous dial: err=%v resp=%v", err, resp)
	}
}

func TestWebSocketDeploymentEvents(t *testing.T) {
	srv, st := newTestServer(t)
	createUser(t, st, "admin", "admin-password-1", store.RoleAdmin)
	admin := login(t, srv.URL, "admin", "admin-password-1")
	var p api.Project
	admin.do("POST", "/api/v1/projects", projectJSON, &p)

	conn := dialWS(t, srv.URL, admin)
	send(t, conn, `{"op":"subscribe","topic":"events"}`)
	time.Sleep(100 * time.Millisecond) // let the subscription land before triggering

	var d api.Deployment
	if code := admin.do("POST", fmt.Sprintf("/api/v1/projects/%d/deployments", p.ID), `{"ref":"main"}`, &d); code != 201 || d.Ref != "refs/heads/main" {
		t.Fatalf("manual deploy: %d %+v", code, d)
	}

	// The deployment fails (unwritable work_dir); wait for the final event.
	readUntil(t, conn, func(m wsMsg) bool {
		var dep api.Deployment
		json.Unmarshal(m.Data, &dep)
		return m.Type == "deployment.updated" && dep.ID == d.ID && dep.Status == "failed"
	})

	send(t, conn, fmt.Sprintf(`{"op":"subscribe","topic":"deployment:%d:log","offset":0}`, d.ID))
	var log strings.Builder
	msgs := readUntil(t, conn, func(m wsMsg) bool { return m.Type == "log.end" })
	for _, m := range msgs {
		if m.Type == "log.chunk" {
			var b []byte
			json.Unmarshal(m.Data, &b)
			log.Write(b)
		}
	}
	// /srv/shop is not writable in tests, so the deployment fails while preparing it.
	if l := log.String(); !strings.Contains(l, "Deployment #1 of Shop") || !strings.Contains(l, "✖") {
		t.Fatalf("backfilled log:\n%s", l)
	}

	// Subscribing to a deployment that does not exist reports an error.
	send(t, conn, `{"op":"subscribe","topic":"deployment:9999:log"}`)
	readUntil(t, conn, func(m wsMsg) bool { return m.Type == "error" })

	var got api.Deployment
	admin.do("GET", fmt.Sprintf("/api/v1/deployments/%d", d.ID), "", &got)
	if got.Status != "failed" || got.TriggeredBy != "admin" || got.FinishedAt == nil {
		t.Fatalf("deployment = %+v", got)
	}
	if code := admin.do("POST", fmt.Sprintf("/api/v1/deployments/%d/cancel", d.ID), "", nil); code != http.StatusConflict {
		t.Fatalf("cancel finished: %d", code)
	}
	var re api.Deployment
	if code := admin.do("POST", fmt.Sprintf("/api/v1/deployments/%d/redeploy", d.ID), "", &re); code != 201 || re.Trigger != "redeploy" || re.Ref != d.Ref {
		t.Fatalf("redeploy: %d %+v", code, re)
	}
	if code := admin.do("POST", fmt.Sprintf("/api/v1/projects/%d/deployments", p.ID), `{"ref":"refs/heads/../x"}`, nil); code != 422 {
		t.Fatalf("bad ref: %d", code)
	}
}

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}
