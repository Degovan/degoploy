package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/degovan/degoploy/internal/audit"
	"github.com/degovan/degoploy/internal/httpx"
	"github.com/degovan/degoploy/internal/store"
)

// auditFilter reads the shared filter parameters: action (prefix), user, q, and
// sensitive=1.
func auditFilter(r *http.Request) store.AuditFilter {
	q := r.URL.Query()
	f := store.AuditFilter{ActionPrefix: q.Get("action"), Query: strings.TrimSpace(q.Get("q"))}
	f.UserID, _ = strconv.ParseInt(q.Get("user"), 10, 64)
	if q.Get("sensitive") == "1" {
		f.Actions = audit.SensitiveActions()
	}
	return f
}

// listAudit returns a page of entries, newest first. tz is the viewer's offset
// from UTC in minutes (east positive) and only affects day_counts.
func (a *API) listAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := auditFilter(r)
	f.BeforeID, _ = strconv.ParseInt(q.Get("before"), 10, 64)
	f.Limit, _ = strconv.Atoi(q.Get("limit"))
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	tzMinutes, _ := strconv.Atoi(q.Get("tz"))
	if tzMinutes < -14*60 || tzMinutes > 14*60 {
		tzMinutes = 0
	}
	offset := time.Duration(tzMinutes) * time.Minute

	// Fetch one extra row to know whether another page exists.
	want := f.Limit
	f.Limit++
	entries, err := a.store.ListAudit(r.Context(), f)
	if err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	page := AuditPage{Entries: make([]AuditEntry, 0, len(entries)), DayCounts: map[string]int{}}
	if len(entries) > want {
		entries = entries[:want]
		next := entries[want-1].ID
		page.NextBefore = &next
	}
	var days []string
	for _, e := range entries {
		var detail map[string]any
		if json.Unmarshal([]byte(e.Detail), &detail) != nil || detail == nil {
			detail = map[string]any{}
		}
		page.Entries = append(page.Entries, AuditEntry{
			ID: e.ID, UserID: e.UserID, Username: e.Username, Action: e.Action, TargetType: e.TargetType,
			TargetID: e.TargetID, Detail: detail, IP: e.IP, CreatedAt: e.CreatedAt.UTC().Format(time.RFC3339),
			Sensitive: audit.IsSensitive(e.Action),
		})
		if day := e.CreatedAt.UTC().Add(offset).Format(time.DateOnly); len(days) == 0 || days[len(days)-1] != day {
			days = append(days, day)
		}
	}

	f.BeforeID, f.Limit = 0, 0
	if page.Total, err = a.store.CountAudit(r.Context(), f); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	if page.DayCounts, err = a.store.AuditDayCounts(r.Context(), f, offset, days); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}

// auditSummary counts all entries, and the sensitive ones, since ?since (RFC 3339).
func (a *API) auditSummary(w http.ResponseWriter, r *http.Request) {
	since, err := time.Parse(time.RFC3339, r.URL.Query().Get("since"))
	if err != nil {
		validationError(w, "since", "must be an RFC 3339 timestamp")
		return
	}
	var out AuditSummary
	f := store.AuditFilter{Since: since}
	if out.Events, err = a.store.CountAudit(r.Context(), f); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	f.Actions = audit.SensitiveActions()
	if out.Sensitive, err = a.store.CountAudit(r.Context(), f); err != nil {
		httpx.InternalError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}
