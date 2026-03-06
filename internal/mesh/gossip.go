package mesh

import (
	"encoding/binary"
	"log/slog"
	"net"
	"time"

	"github.com/vmihailenco/msgpack/v5"
	"notashelf.dev/ncro/internal/cache"
)

const maxPacketSize = 65536 // UDP max payload

// Wire format: [2-byte sig length][sig bytes][msgpack body]
func encodePacket(node *Node, msg Message) ([]byte, error) {
	body, sig, err := node.Sign(msg)
	if err != nil {
		return nil, err
	}
	pkt := make([]byte, 2+len(sig)+len(body))
	binary.BigEndian.PutUint16(pkt[:2], uint16(len(sig)))
	copy(pkt[2:], sig)
	copy(pkt[2+len(sig):], body)
	return pkt, nil
}

func decodePacket(pkt []byte) (Message, []byte, []byte, bool) {
	if len(pkt) < 2 {
		return Message{}, nil, nil, false
	}
	sigLen := int(binary.BigEndian.Uint16(pkt[:2]))
	if len(pkt) < 2+sigLen {
		return Message{}, nil, nil, false
	}
	sig := pkt[2 : 2+sigLen]
	body := pkt[2+sigLen:]
	var msg Message
	if err := msgpack.Unmarshal(body, &msg); err != nil {
		return Message{}, nil, nil, false
	}
	return msg, body, sig, true
}

// Starts a UDP listener at addr. Received route announcements are merged into store.
// Blocks until the conn is closed; call in a goroutine.
func ListenAndServe(addr string, store *RouteStore) error {
	conn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return err
	}
	go func() {
		defer conn.Close()
		buf := make([]byte, maxPacketSize)
		for {
			n, src, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			msg, _, _, ok := decodePacket(buf[:n])
			if !ok {
				slog.Warn("mesh: malformed packet", "src", src)
				continue
			}
			if msg.Type == MsgAnnounce && len(msg.Routes) > 0 {
				store.Merge(msg.Routes)
				slog.Debug("mesh: merged peer routes", "node", msg.NodeID, "src", src, "count", len(msg.Routes))
			}
		}
	}()
	return nil
}

// Sends an MsgAnnounce carrying routes to a single peer address.
func Announce(peerAddr string, node *Node, routes []cache.RouteEntry) error {
	msg := Message{
		Type:      MsgAnnounce,
		NodeID:    node.ID(),
		Timestamp: time.Now().UnixNano(),
		Routes:    routes,
	}
	pkt, err := encodePacket(node, msg)
	if err != nil {
		return err
	}
	addr, err := net.ResolveUDPAddr("udp", peerAddr)
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	_, err = conn.Write(pkt)
	return err
}

// RouteSource retrieves routes to gossip.
type RouteSource interface {
	ListRecentRoutes(n int) ([]cache.RouteEntry, error)
}

// Announces our top routes to each peer on interval. Blocks until stop is closed.
func RunGossipLoop(node *Node, src RouteSource, peers []string, interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			routes, err := src.ListRecentRoutes(100)
			if err != nil {
				slog.Warn("mesh: failed to list routes for gossip", "error", err)
				continue
			}
			if len(routes) == 0 {
				continue
			}
			for _, peer := range peers {
				if err := Announce(peer, node, routes); err != nil {
					slog.Warn("mesh: announce failed", "peer", peer, "error", err)
				}
			}
			slog.Debug("mesh: announced routes to peers", "routes", len(routes), "peers", len(peers))
		}
	}
}
