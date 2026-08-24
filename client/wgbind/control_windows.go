//go:build windows

package wgbind

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// sioUDPConnReset = _WSAIOW(IOC_VENDOR, 12). Setting it to 0 stops a UDP socket
// from reporting WSAECONNRESET when a prior send elicited an ICMP port-
// unreachable — which otherwise kills the wireguard-go receive routine.
const sioUDPConnReset uint32 = 0x9800000C

func controlDisableConnReset(network, address string, c syscall.RawConn) error {
	var ioctlErr error
	if err := c.Control(func(fd uintptr) {
		var flag uint32 // 0 = FALSE (disable connection-reset behaviour)
		var bytesReturned uint32
		ioctlErr = windows.WSAIoctl(
			windows.Handle(fd),
			sioUDPConnReset,
			(*byte)(unsafe.Pointer(&flag)),
			uint32(unsafe.Sizeof(flag)),
			nil, 0,
			&bytesReturned, nil, 0,
		)
	}); err != nil {
		return err
	}
	return ioctlErr
}

func isConnReset(err error) bool {
	return errors.Is(err, windows.WSAECONNRESET)
}
