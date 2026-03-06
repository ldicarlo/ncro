package mesh_test

import (
	"net"
	"testing"
	"time"

	"notashelf.dev/ncro/internal/cache"
	"notashelf.dev/ncro/internal/mesh"
)

func TestAnnounceAndReceive(t *testing.T) {
	store := mesh.NewRouteStore()
	node, err := mesh.NewNode("", store)
	if err != nil {
		t.Fatal(err)
	}

	// Bind to an ephemeral port.
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := conn.LocalAddr().String()
	conn.Close()

	if err := mesh.ListenAndServe(addr, store); err != nil {
		t.Fatalf("ListenAndServe: %v", err)
	}

	routes := []cache.RouteEntry{
		{
			StorePath:   "test-pkg-abc",
			UpstreamURL: "https://cache.nixos.org",
			LatencyEMA:  25,
			TTL:         time.Now().Add(time.Hour),
		},
	}

	if err := mesh.Announce(addr, node, routes); err != nil {
		t.Fatalf("Announce: %v", err)
	}

	// Give the listener goroutine time to process the packet.
	time.Sleep(50 * time.Millisecond)

	entry := store.Get("test-pkg-abc")
	if entry == nil {
		t.Fatal("route not merged into store after announce")
	}
	if entry.UpstreamURL != "https://cache.nixos.org" {
		t.Errorf("UpstreamURL = %q", entry.UpstreamURL)
	}
}
