// Package webhook verifies and normalises GitHub and Gitea webhook deliveries.
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"

	"github.com/degovan/degoploy/internal/trigger"
)

const (
	ProviderGitHub = "github"
	ProviderGitea  = "gitea"
)

var (
	ErrNoSignature  = errors.New("missing signature header")
	ErrBadSignature = errors.New("signature does not match the webhook secret")
)

// Delivery is a parsed webhook request.
type Delivery struct {
	Provider   string
	Event      string // event header value, e.g. "push", "ping"
	DeliveryID string
	Push       *Push // set for push events only
}

// Push describes a push event in provider-neutral form.
type Push struct {
	Ref     string // full ref, e.g. refs/heads/main
	Before  string
	After   string
	Deleted bool // the ref was deleted; nothing to deploy
	Author  string
	Trigger trigger.Push
}

// Verify checks the HMAC-SHA256 signature of body against secret.
//
// GitHub sends X-Hub-Signature-256: sha256=<hex>. Gitea sends X-Gitea-Signature: <hex>
// and, in recent versions, the GitHub header as well; either is accepted for Gitea.
func Verify(provider string, h http.Header, body []byte, secret string) error {
	sig := strings.TrimPrefix(h.Get("X-Hub-Signature-256"), "sha256=")
	if sig == "" && provider == ProviderGitea {
		sig = h.Get("X-Gitea-Signature")
	}
	if sig == "" {
		return ErrNoSignature
	}
	got, err := hex.DecodeString(strings.TrimSpace(sig))
	if err != nil {
		return ErrBadSignature
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	if !hmac.Equal(got, mac.Sum(nil)) {
		return ErrBadSignature
	}
	return nil
}

// Sign returns the X-Hub-Signature-256 value for body; used by tests and tooling.
func Sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Parse reads the event headers and, for push events, the payload.
// body is the raw request body; form-encoded deliveries (GitHub's default content
// type) carry the JSON in the "payload" field.
func Parse(provider string, h http.Header, body []byte) (*Delivery, error) {
	d := &Delivery{Provider: provider}
	if provider == ProviderGitea {
		d.Event = firstHeader(h, "X-Gitea-Event", "X-Gogs-Event", "X-GitHub-Event")
		d.DeliveryID = firstHeader(h, "X-Gitea-Delivery", "X-Gogs-Delivery", "X-GitHub-Delivery")
	} else {
		d.Event = h.Get("X-GitHub-Event")
		d.DeliveryID = h.Get("X-GitHub-Delivery")
	}
	if d.Event == "" {
		return nil, fmt.Errorf("missing event header (is this a %s webhook?)", provider)
	}
	if d.Event != "push" {
		return d, nil
	}

	payload, err := jsonPayload(h.Get("Content-Type"), body)
	if err != nil {
		return nil, err
	}
	var p pushPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil, fmt.Errorf("decode push payload: %w", err)
	}
	d.Push = p.normalize()
	return d, nil
}

func jsonPayload(contentType string, body []byte) ([]byte, error) {
	mt, _, _ := mime.ParseMediaType(contentType)
	if mt != "application/x-www-form-urlencoded" {
		return body, nil
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, fmt.Errorf("decode form payload: %w", err)
	}
	p := form.Get("payload")
	if p == "" {
		return nil, errors.New("form-encoded delivery without a payload field")
	}
	return []byte(p), nil
}

type commit struct {
	ID       string   `json:"id"`
	Message  string   `json:"message"`
	Added    []string `json:"added"`
	Removed  []string `json:"removed"`
	Modified []string `json:"modified"`
	Author   person   `json:"author"`
}

type person struct {
	Name     string `json:"name"`
	Username string `json:"username"`
	Login    string `json:"login"`
}

// pushPayload covers the fields GitHub and Gitea have in common.
type pushPayload struct {
	Ref          string   `json:"ref"`
	Before       string   `json:"before"`
	After        string   `json:"after"`
	Created      bool     `json:"created"`
	Deleted      bool     `json:"deleted"`
	Forced       bool     `json:"forced"`
	Commits      []commit `json:"commits"`
	TotalCommits int      `json:"total_commits"` // Gitea only
	HeadCommit   *commit  `json:"head_commit"`
	Pusher       person   `json:"pusher"`
}

// GitHub lists at most this many commits in a push payload.
const githubCommitLimit = 2048

func (p *pushPayload) normalize() *Push {
	out := &Push{Ref: p.Ref, Before: p.Before, After: p.After, Deleted: p.Deleted || isZeroSHA(p.After)}

	switch {
	case strings.HasPrefix(p.Ref, "refs/heads/"):
		out.Trigger.Kind, out.Trigger.RefName = trigger.KindPush, strings.TrimPrefix(p.Ref, "refs/heads/")
	case strings.HasPrefix(p.Ref, "refs/tags/"):
		out.Trigger.Kind, out.Trigger.RefName = trigger.KindTag, strings.TrimPrefix(p.Ref, "refs/tags/")
	default:
		out.Trigger.Kind, out.Trigger.RefName = "", p.Ref
	}

	head := p.HeadCommit
	if head == nil && len(p.Commits) > 0 {
		head = &p.Commits[len(p.Commits)-1]
	}
	if head != nil {
		out.Trigger.Message = head.Message
		out.Author = head.Author.Name
	}
	if out.Author == "" {
		out.Author = firstNonEmpty(p.Pusher.Name, p.Pusher.Username, p.Pusher.Login)
	}

	seen := map[string]bool{}
	for _, c := range p.Commits {
		for _, list := range [][]string{c.Added, c.Removed, c.Modified} {
			for _, f := range list {
				if !seen[f] {
					seen[f] = true
					out.Trigger.Files = append(out.Trigger.Files, f)
				}
			}
		}
	}
	truncated := len(p.Commits) >= githubCommitLimit || (p.TotalCommits > 0 && p.TotalCommits > len(p.Commits))
	out.Trigger.FilesKnown = len(p.Commits) > 0 && !p.Created && !p.Forced && !truncated
	return out
}

func isZeroSHA(s string) bool {
	return s != "" && strings.Trim(s, "0") == ""
}

func firstHeader(h http.Header, names ...string) string {
	for _, n := range names {
		if v := h.Get(n); v != "" {
			return v
		}
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
