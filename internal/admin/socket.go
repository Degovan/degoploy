package admin

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/user"
	"strconv"
	"sync"
	"time"
)

// maxSocketPath is the longest Unix socket path every platform accepts.
const maxSocketPath = 104

// Listen opens the admin socket at path, replacing a stale one left behind by
// a crash. The socket is readable and writable by the service's user and group.
func Listen(path string) (net.Listener, error) {
	if len(path) > maxSocketPath {
		return nil, fmt.Errorf("admin socket path is %d bytes, longer than %d; set admin_socket to a shorter path", len(path), maxSocketPath)
	}
	if c, err := net.DialTimeout("unix", path, time.Second); err == nil {
		c.Close()
		return nil, fmt.Errorf("%s is in use; is another degoploy running?", path)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	// Until this chmod the umask leaves group and others without write
	// permission, which connecting needs, so nobody else can get in early.
	if err := os.Chmod(path, 0o660); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

// Serve answers admin requests on l until ctx ends.
func Serve(ctx context.Context, l net.Listener, h http.Handler) error {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return WithPeer(ctx, peerOf(c))
		},
	}
	stop := context.AfterFunc(ctx, func() { srv.Close() })
	defer stop()
	if err := srv.Serve(l); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Peer is the Linux account on the other end of the socket.
type Peer struct {
	UID  int
	Name string
}

type peerKey struct{}

func WithPeer(ctx context.Context, p Peer) context.Context {
	return context.WithValue(ctx, peerKey{}, p)
}

func PeerFrom(ctx context.Context) Peer {
	if p, ok := ctx.Value(peerKey{}).(Peer); ok {
		return p
	}
	return Peer{UID: -1, Name: "unknown"}
}

// CurrentPeer is the account running this process.
func CurrentPeer() Peer {
	uid := os.Getuid()
	return Peer{UID: uid, Name: userName(uid)}
}

func userName(uid int) string {
	if uid < 0 {
		return "unknown"
	}
	if u, err := user.LookupId(strconv.Itoa(uid)); err == nil {
		return u.Username
	}
	return "uid " + strconv.Itoa(uid)
}

// pipeListener hands out in-memory connections, so the CLI can run the same
// handler in-process when the service is not running.
type pipeListener struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
}

func newPipeListener() *pipeListener {
	return &pipeListener{conns: make(chan net.Conn), done: make(chan struct{})}
}

func (l *pipeListener) Accept() (net.Conn, error) {
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *pipeListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return nil
}

func (l *pipeListener) Addr() net.Addr { return pipeAddr{} }

func (l *pipeListener) dial(ctx context.Context) (net.Conn, error) {
	client, server := net.Pipe()
	select {
	case l.conns <- server:
		return client, nil
	case <-l.done:
		return nil, net.ErrClosed
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

type pipeAddr struct{}

func (pipeAddr) Network() string { return "pipe" }
func (pipeAddr) String() string  { return "in-process" }
