package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/degovan/degoploy/internal/httpx"
)

// Client calls the admin API, over the socket of a running service or
// in-process against the data directory.
type Client struct {
	http  *http.Client
	close func()
	// Socket is the path it is connected to; empty when in-process.
	Socket string
}

// Dial connects to the admin socket at path. Errors wrap the dial error, so
// callers can tell a missing service (fs.ErrNotExist, syscall.ECONNREFUSED)
// from a denied one (fs.ErrPermission).
func Dial(path string) (*Client, error) {
	c, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return nil, err
	}
	c.Close()
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "unix", path)
		},
	}
	return &Client{http: &http.Client{Transport: tr}, close: tr.CloseIdleConnections, Socket: path}, nil
}

// InProcess serves h in this process for the returned client; cleanup runs on Close.
func InProcess(h http.Handler, cleanup func()) *Client {
	l := newPipeListener()
	ctx, cancel := context.WithCancel(context.Background())
	go Serve(ctx, l, h)
	tr := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) { return l.dial(ctx) },
	}
	return &Client{http: &http.Client{Transport: tr}, close: func() {
		tr.CloseIdleConnections()
		cancel()
		l.Close()
		cleanup()
	}}
}

func (c *Client) Close() { c.close() }

// Error is a refusal from the admin API.
type Error struct {
	Status int
	Code   string
	Field  string
	Msg    string
}

func (e *Error) Error() string { return e.Msg }

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://degoploy"+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var e httpx.ErrorBody
		if json.NewDecoder(resp.Body).Decode(&e) != nil || e.Error == "" {
			e.Error = resp.Status
		}
		return &Error{Status: resp.StatusCode, Code: e.Code, Field: e.Field, Msg: e.Error}
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) Status(ctx context.Context) (Status, error) {
	var s Status
	return s, c.do(ctx, http.MethodGet, "/v1/status", nil, &s)
}

func (c *Client) Users(ctx context.Context) ([]User, error) {
	var us []User
	return us, c.do(ctx, http.MethodGet, "/v1/users", nil, &us)
}

func (c *Client) CreateUser(ctx context.Context, req CreateUserRequest) (User, error) {
	var u User
	return u, c.do(ctx, http.MethodPost, "/v1/users", req, &u)
}

func (c *Client) UpdateUser(ctx context.Context, username string, req UpdateUserRequest) (User, error) {
	var u User
	return u, c.do(ctx, http.MethodPatch, "/v1/users/"+url.PathEscape(username), req, &u)
}

func (c *Client) SetPassword(ctx context.Context, username string, req SetPasswordRequest) error {
	return c.do(ctx, http.MethodPut, "/v1/users/"+url.PathEscape(username)+"/password", req, nil)
}

func (c *Client) CheckConfig(ctx context.Context) (Report, error) {
	var r Report
	return r, c.do(ctx, http.MethodGet, "/v1/config/check", nil, &r)
}

// Backup streams an archive into w and returns how many files it holds.
func (c *Client) Backup(ctx context.Context, w io.Writer, logs bool) (int, error) {
	path := "/v1/backup"
	if !logs {
		path += "?logs=0"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://degoploy"+path, nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("backup: %s", resp.Status)
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		return 0, err
	}
	if msg := resp.Trailer.Get(trailerError); msg != "" {
		return 0, errors.New(msg)
	}
	n, err := strconv.Atoi(resp.Trailer.Get(trailerFiles))
	if err != nil {
		return 0, errors.New("backup ended early; the archive is incomplete")
	}
	return n, nil
}
