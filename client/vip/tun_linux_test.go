//go:build linux

package vip

import (
	"encoding/binary"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	wgtun "golang.zx2c4.com/wireguard/tun"
)

// The translation layer edits live packets and repairs their checksums. The
// other tests check that against the same arithmetic that produced it; this one
// checks it against an operating system, which silently drops anything it
// disagrees with. A reply the kernel accepts is the evidence.
//
// It creates a TUN device, so it needs root and is opt-in:
//
//	sudo -E env "PATH=$PATH" ZWAN_TUN_TEST=1 go test ./client/vip/ -run RealTun -v
const (
	probeIface     = "zwanvip0"
	probeLocalSelf = "100.112.0.1"
	probeLocalPeer = "100.112.0.2"
	probeRealSelf  = "100.64.0.1"
	probeRealPeer  = "100.64.0.2"
	probePort      = 9999

	// wireguard-go leaves room at the front of every buffer; the Linux TUN
	// needs at least the virtio header there on write.
	probeOffset = 16
)

// seen records what the responder observed, so the test can assert that packets
// reached it already translated into overlay space.
type seen struct {
	icmp, udp, tcp int
	wrongAddresses int
}

func TestRealTunDeviceRoundTrip(t *testing.T) {
	if os.Getenv("ZWAN_TUN_TEST") == "" {
		t.Skip("set ZWAN_TUN_TEST=1 and run as root; this creates a TUN device")
	}
	if os.Geteuid() != 0 {
		t.Fatal("this test needs root to create a TUN device")
	}

	inner, err := wgtun.CreateTUN(probeIface, 1420)
	if err != nil {
		t.Fatalf("create TUN: %v", err)
	}
	defer inner.Close()
	name, err := inner.Name()
	if err != nil {
		t.Fatal(err)
	}

	table, err := NewTable(netip.MustParsePrefix("100.112.0.0/24"), netip.MustParsePrefix("100.64.0.0/24"))
	if err != nil {
		t.Fatal(err)
	}
	dev, self, err := Wrap(inner, table, netip.MustParseAddr(probeRealSelf))
	if err != nil {
		t.Fatal(err)
	}
	if self.String() != probeLocalSelf {
		t.Fatalf("this device's local address = %v, want %s", self, probeLocalSelf)
	}
	// The far side has to be known before its first packet, exactly as the
	// engine maps every peer when it programs routes.
	if peer, ok := table.Local(netip.MustParseAddr(probeRealPeer)); !ok || peer.String() != probeLocalPeer {
		t.Fatalf("peer local address = %v (ok=%v), want %s", peer, ok, probeLocalPeer)
	}

	ipCmd(t, "addr", "add", probeLocalSelf+"/24", "dev", name)
	ipCmd(t, "link", "set", name, "up")

	got := &seen{}
	stop := make(chan struct{})
	done := make(chan struct{})
	go respond(dev, got, stop, done)
	defer func() {
		close(stop)
		_ = inner.Close() // unblocks the read loop
		<-done
	}()

	t.Run("icmp", func(t *testing.T) {
		out, err := exec.Command("ping", "-c", "2", "-W", "3", probeLocalPeer).CombinedOutput()
		if err != nil {
			t.Fatalf("the kernel did not accept our replies: %v\n%s", err, out)
		}
	})

	t.Run("udp", func(t *testing.T) {
		c, err := net.Dial("udp", net.JoinHostPort(probeLocalPeer, strconv.Itoa(probePort)))
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		if _, err := c.Write([]byte("ping over udp")); err != nil {
			t.Fatal(err)
		}
		buf := make([]byte, 128)
		_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
		n, err := c.Read(buf)
		if err != nil {
			t.Fatalf("no datagram came back, so the kernel rejected the checksum: %v", err)
		}
		if string(buf[:n]) != "ping over udp" {
			t.Fatalf("datagram came back as %q", buf[:n])
		}
	})

	t.Run("tcp", func(t *testing.T) {
		// The responder answers a SYN with a reset. A reset the kernel accepts
		// refuses the connection immediately; a broken checksum would be dropped
		// and this would sit until the deadline instead.
		start := time.Now()
		c, err := net.DialTimeout("tcp", net.JoinHostPort(probeLocalPeer, strconv.Itoa(probePort)), 4*time.Second)
		if err == nil {
			c.Close()
			t.Fatal("the connection should have been refused")
		}
		if time.Since(start) > 3*time.Second {
			t.Fatalf("connect took %v: the reset was not accepted", time.Since(start))
		}
	})

	if got.wrongAddresses != 0 {
		t.Fatalf("%d packets reached the tunnel side still in local addresses", got.wrongAddresses)
	}
	if got.icmp == 0 || got.udp == 0 || got.tcp == 0 {
		t.Fatalf("responder saw icmp=%d udp=%d tcp=%d, want all three", got.icmp, got.udp, got.tcp)
	}
	t.Logf("translated packets seen: icmp=%d udp=%d tcp=%d", got.icmp, got.udp, got.tcp)
}

// respond plays the far side of the tunnel. Everything it reads is in overlay
// space and everything it writes is too; the wrapper does the rest.
func respond(dev *Device, got *seen, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)

	bufs := make([][]byte, dev.BatchSize())
	for i := range bufs {
		bufs[i] = make([]byte, probeOffset+2048)
	}
	sizes := make([]int, len(bufs))

	for {
		select {
		case <-stop:
			return
		default:
		}
		n, err := dev.Read(bufs, sizes, probeOffset)
		if err != nil {
			return
		}
		for i := 0; i < n; i++ {
			if sizes[i] == 0 {
				continue
			}
			pkt := bufs[i][probeOffset : probeOffset+sizes[i]]
			reply := answer(pkt, got)
			if reply == nil {
				continue
			}
			out := make([]byte, probeOffset+len(reply))
			copy(out[probeOffset:], reply)
			_, _ = dev.Write([][]byte{out}, probeOffset)
		}
	}
}

// answer builds the far side's reply to one packet, or nil to ignore it. All the
// checksums here are computed from scratch, so they do not share arithmetic with
// the incremental update being tested.
func answer(pkt []byte, got *seen) []byte {
	if len(pkt) < 20 || pkt[0]>>4 != 4 {
		return nil
	}
	if addrAt(pkt[12:16]).String() != probeRealSelf || addrAt(pkt[16:20]).String() != probeRealPeer {
		got.wrongAddresses++
		return nil
	}
	reply := append([]byte(nil), pkt...)
	copy(reply[12:16], pkt[16:20]) // swap the addresses back
	copy(reply[16:20], pkt[12:16])

	l4 := reply[20:]
	switch reply[9] {
	case protoICMP:
		if len(l4) < 8 || l4[0] != 8 { // echo request
			return nil
		}
		got.icmp++
		l4[0] = 0 // echo reply
		put16(l4[2:4], 0)
		put16(l4[2:4], checksumOf(l4))

	case protoUDP:
		if len(l4) < 8 {
			return nil
		}
		got.udp++
		src, dst := get16(l4[0:2]), get16(l4[2:4])
		put16(l4[0:2], dst)
		put16(l4[2:4], src)
		put16(l4[6:8], 0)
		put16(l4[6:8], pseudoChecksum(reply))

	case protoTCP:
		if len(l4) < 20 || l4[13]&0x02 == 0 { // SYN
			return nil
		}
		got.tcp++
		src, dst := get16(l4[0:2]), get16(l4[2:4])
		seq := binary.BigEndian.Uint32(l4[4:8])

		rst := make([]byte, 20)
		put16(rst[0:2], dst)
		put16(rst[2:4], src)
		binary.BigEndian.PutUint32(rst[8:12], seq+1) // acknowledge the SYN
		rst[12] = 0x50                               // header length
		rst[13] = 0x14                               // RST | ACK
		reply = append(reply[:20], rst...)
		binary.BigEndian.PutUint16(reply[2:4], uint16(len(reply)))
		put16(reply[20+16:20+18], pseudoChecksum(reply))

	default:
		return nil
	}

	put16(reply[10:12], 0)
	put16(reply[10:12], checksumOf(reply[:20]))
	return reply
}

// checksumOf is the ones-complement checksum of a block.
func checksumOf(b []byte) uint16 { return checksum(b) }

// pseudoChecksum computes a TCP/UDP checksum over the pseudo-header.
func pseudoChecksum(pkt []byte) uint16 {
	l4 := pkt[20:]
	buf := make([]byte, 12+len(l4))
	copy(buf[0:4], pkt[12:16])
	copy(buf[4:8], pkt[16:20])
	buf[9] = pkt[9]
	binary.BigEndian.PutUint16(buf[10:12], uint16(len(l4)))
	copy(buf[12:], l4)
	return checksum(buf)
}

func ipCmd(t *testing.T, args ...string) {
	t.Helper()
	if out, err := exec.Command("ip", args...).CombinedOutput(); err != nil {
		t.Fatalf("ip %v: %v: %s", args, err, out)
	}
}
