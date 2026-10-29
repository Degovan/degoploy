package admin

import (
	"net"

	"golang.org/x/sys/unix"
)

// peerOf asks the kernel which user opened a Unix socket connection; in-process
// connections belong to this process's user.
func peerOf(c net.Conn) Peer {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return CurrentPeer()
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return Peer{UID: -1, Name: "unknown"}
	}
	var cred *unix.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil || credErr != nil {
		return Peer{UID: -1, Name: "unknown"}
	}
	uid := int(cred.Uid)
	return Peer{UID: uid, Name: userName(uid)}
}
