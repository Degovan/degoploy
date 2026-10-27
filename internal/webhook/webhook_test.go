package webhook

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/degovan/degoploy/internal/trigger"
)

const githubPush = `{
  "ref": "refs/heads/main",
  "before": "1111111111111111111111111111111111111111",
  "after": "2222222222222222222222222222222222222222",
  "created": false, "deleted": false, "forced": false,
  "commits": [
    {"id": "aaa", "message": "first", "added": ["src/a.go"], "removed": [], "modified": ["README.md"], "author": {"name": "Ana"}},
    {"id": "2222222222222222222222222222222222222222", "message": "second\n\nbody", "added": [], "removed": ["old.txt"], "modified": ["src/a.go"], "author": {"name": "Budi"}}
  ],
  "head_commit": {"id": "2222222222222222222222222222222222222222", "message": "second\n\nbody", "author": {"name": "Budi", "username": "budi"}},
  "pusher": {"name": "budi"}
}`

// Gitea omits created/forced and reports total_commits; here it truncated the list.
const giteaTagPush = `{
  "ref": "refs/tags/v1.2.0",
  "before": "0000000000000000000000000000000000000000",
  "after": "3333333333333333333333333333333333333333",
  "commits": [],
  "total_commits": 0,
  "head_commit": {"id": "3333333333333333333333333333333333333333", "message": "release", "author": {"name": "Citra"}},
  "pusher": {"login": "citra", "username": "citra"}
}`

func headers(kv ...string) http.Header {
	h := http.Header{}
	for i := 0; i < len(kv); i += 2 {
		h.Set(kv[i], kv[i+1])
	}
	return h
}

func TestVerify(t *testing.T) {
	body := []byte(githubPush)
	sig := Sign(body, "s3cret")

	if err := Verify(ProviderGitHub, headers("X-Hub-Signature-256", sig), body, "s3cret"); err != nil {
		t.Fatalf("valid github signature: %v", err)
	}
	if err := Verify(ProviderGitHub, headers("X-Hub-Signature-256", sig), body, "other"); err != ErrBadSignature {
		t.Fatalf("wrong secret: %v", err)
	}
	if err := Verify(ProviderGitHub, headers(), body, "s3cret"); err != ErrNoSignature {
		t.Fatalf("missing header: %v", err)
	}
	// GitHub must not accept Gitea's header.
	if err := Verify(ProviderGitHub, headers("X-Gitea-Signature", strings.TrimPrefix(sig, "sha256=")), body, "s3cret"); err != ErrNoSignature {
		t.Fatalf("github with gitea header: %v", err)
	}
	if err := Verify(ProviderGitea, headers("X-Gitea-Signature", strings.TrimPrefix(sig, "sha256=")), body, "s3cret"); err != nil {
		t.Fatalf("valid gitea signature: %v", err)
	}
	tampered := append([]byte{}, body...)
	tampered[10] = 'X'
	if err := Verify(ProviderGitea, headers("X-Gitea-Signature", strings.TrimPrefix(sig, "sha256=")), tampered, "s3cret"); err != ErrBadSignature {
		t.Fatalf("tampered body: %v", err)
	}
}

func TestParseGitHubPush(t *testing.T) {
	d, err := Parse(ProviderGitHub, headers("X-GitHub-Event", "push", "X-GitHub-Delivery", "d-1", "Content-Type", "application/json"), []byte(githubPush))
	if err != nil {
		t.Fatal(err)
	}
	p := d.Push
	if d.DeliveryID != "d-1" || p == nil || p.Trigger.Kind != trigger.KindPush || p.Trigger.RefName != "main" {
		t.Fatalf("delivery = %+v push = %+v", d, p)
	}
	if p.Author != "Budi" || !strings.HasPrefix(p.Trigger.Message, "second") || !p.Trigger.FilesKnown {
		t.Fatalf("push = %+v", p)
	}
	if got := strings.Join(p.Trigger.Files, ","); got != "src/a.go,README.md,old.txt" {
		t.Fatalf("files = %s", got)
	}
}

func TestParseFormEncoded(t *testing.T) {
	body := []byte("payload=" + url.QueryEscape(githubPush))
	d, err := Parse(ProviderGitHub, headers("X-GitHub-Event", "push", "Content-Type", "application/x-www-form-urlencoded"), body)
	if err != nil || d.Push == nil || d.Push.Trigger.RefName != "main" {
		t.Fatalf("form payload: %+v %v", d, err)
	}
}

func TestParseGiteaTag(t *testing.T) {
	d, err := Parse(ProviderGitea, headers("X-Gitea-Event", "push", "X-Gitea-Delivery", "g-1"), []byte(giteaTagPush))
	if err != nil {
		t.Fatal(err)
	}
	p := d.Push
	if p.Trigger.Kind != trigger.KindTag || p.Trigger.RefName != "v1.2.0" || p.Author != "Citra" || p.Trigger.FilesKnown {
		t.Fatalf("push = %+v", p)
	}
}

func TestParseDeleteAndPing(t *testing.T) {
	body := strings.Replace(githubPush, `"after": "2222222222222222222222222222222222222222"`, `"after": "0000000000000000000000000000000000000000"`, 1)
	d, err := Parse(ProviderGitHub, headers("X-GitHub-Event", "push"), []byte(body))
	if err != nil || !d.Push.Deleted {
		t.Fatalf("deleted ref not detected: %+v %v", d.Push, err)
	}
	d, err = Parse(ProviderGitHub, headers("X-GitHub-Event", "ping"), []byte(`{"zen":"x"}`))
	if err != nil || d.Event != "ping" || d.Push != nil {
		t.Fatalf("ping: %+v %v", d, err)
	}
	if _, err := Parse(ProviderGitea, headers(), []byte(`{}`)); err == nil {
		t.Fatal("missing event header must fail")
	}
}
