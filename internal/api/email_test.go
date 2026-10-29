package api_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/degovan/degoploy/internal/api"
	"github.com/degovan/degoploy/internal/config"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/mail/smtptest"
	"github.com/degovan/degoploy/internal/store"
)

func TestEmailSettings(t *testing.T) {
	mailbox := smtptest.New(t)
	path := filepath.Join(t.TempDir(), "degoploy.yaml")
	os.WriteFile(path, []byte("# settings\nlisten: \":8080\"\n"), 0o600)
	srv, st := newTestServerWith(t, func(cfg *config.Config, _ string) {
		cfg.Path = path
		cfg.Locked = map[string]string{"smtp.from_name": "${MAIL_NAME}"}
	})
	createUser(t, st, "root", "root-password-1", store.RoleAdmin)
	admin := login(t, srv.URL, "root", "root-password-1")

	var s api.EmailStatus
	if code := admin.do("GET", "/api/v1/email", "", &s); code != 200 || s.Configured || s.Ready || s.LastTest != nil {
		t.Fatalf("initial: %d %+v", code, s)
	}
	var e httpx.ErrorBody
	if code := admin.do("POST", "/api/v1/email/test", `{"to":"admin@example.com"}`, &e); code != 409 {
		t.Fatalf("test before configuring: %d", code)
	}
	for _, c := range []struct{ body, field string }{
		{`{"from_name":"Bot"}`, "smtp.from_name"},
		{`{"host":"smtp.example.com","from_address":"nope"}`, "smtp.from_address"},
		{`{"host":"smtp.example.com","port":70000,"from_address":"a@b.c"}`, "smtp.port"},
		{`{"encryption":"ssl"}`, "smtp.encryption"},
	} {
		if code := admin.do("PATCH", "/api/v1/email", c.body, &e); code != 422 || e.Field != c.field {
			t.Fatalf("%s: %d %+v", c.body, code, e)
		}
	}

	body := `{"host":"` + mailbox.Host + `","port":` + strconv.Itoa(mailbox.Port) + `,"encryption":"none",
		"username":"bot","password":"mail-pw","from_address":"no-reply@example.com"}`
	if code := admin.do("PATCH", "/api/v1/email", body, &s); code != 200 || !s.Configured || s.Ready || !s.PasswordSet {
		t.Fatalf("save: %d %+v", code, s)
	}
	raw, _ := os.ReadFile(path)
	if text := string(raw); !strings.Contains(text, "# settings") || !strings.Contains(text, "password: sealed:v1:") ||
		strings.Contains(text, "mail-pw") {
		t.Fatalf("config file:\n%s", text)
	}

	// A delivered test enables invites; changing a setting pauses them again.
	if code := admin.do("POST", "/api/v1/email/test", `{"to":"admin@example.com"}`, &s); code != 200 ||
		!s.Ready || !s.LastTest.OK || !s.LastTest.Current || !strings.HasPrefix(s.LastTest.Detail, "250") {
		t.Fatalf("test: %d %+v %+v", code, s, s.LastTest)
	}
	if msgs := mailbox.Messages(); len(msgs) != 1 || msgs[0].Username != "bot" || msgs[0].Password != "mail-pw" {
		t.Fatalf("sent = %+v", msgs)
	}
	if admin.do("PATCH", "/api/v1/email", `{"password":"new-pw"}`, &s); s.Ready || s.LastTest.Current {
		t.Fatalf("after password change: %+v %+v", s, s.LastTest)
	}

	// A failed test is recorded but does not enable invites.
	mailbox.RejectRcpt = true
	if admin.do("POST", "/api/v1/email/test", `{"to":"admin@example.com"}`, &s); s.Ready || s.LastTest.OK ||
		!strings.Contains(s.LastTest.Detail, "No such user") {
		t.Fatalf("failed test: %+v %+v", s, s.LastTest)
	}

	var page api.AuditPage
	if admin.do("GET", "/api/v1/audit?action=email.", "", &page); page.Total != 4 {
		t.Fatalf("audit total = %d", page.Total)
	}
}
