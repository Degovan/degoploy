//go:build !linux

package admin

import "net"

// peerOf cannot read peer credentials outside Linux; the audit log then shows
// "unknown" for commands that came through the socket.
func peerOf(c net.Conn) Peer {
	if _, ok := c.(*net.UnixConn); ok {
		return Peer{UID: -1, Name: "unknown"}
	}
	return CurrentPeer()
}
