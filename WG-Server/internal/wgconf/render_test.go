package wgconf

import "testing"

// TestRenderClient membandingkan keluaran byte demi byte dengan .conf yang diharapkan,
// termasuk baris opsional yang harus hilang saat kosong.
func TestRenderClient(t *testing.T) {
	got := RenderClient(ClientConfig{
		PrivateKey: "CLIENTPRIV=", Address: "10.8.0.2/32", DNS: []string{"1.1.1.1", "9.9.9.9"},
		ServerPublicKey: "SERVERPUB=", Endpoint: "vpn.example.id:51820",
		AllowedIPs: []string{"0.0.0.0/0"}, PersistentKeepalive: 25,
	})
	want := "[Interface]\nPrivateKey = CLIENTPRIV=\nAddress = 10.8.0.2/32\nDNS = 1.1.1.1, 9.9.9.9\n\n" +
		"[Peer]\nPublicKey = SERVERPUB=\nEndpoint = vpn.example.id:51820\nAllowedIPs = 0.0.0.0/0\nPersistentKeepalive = 25\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestRenderServer: byte-exact, termasuk MTU/PSK yang hilang saat kosong dan peer site-to-site.
func TestRenderServer(t *testing.T) {
	got := RenderServer(ServerConfig{
		PrivateKey: "SERVERPRIV=", Address: "10.8.0.1/24", ListenPort: 51820,
		Peers: []ServerPeer{
			{PublicKey: "PEERA=", PresharedKey: "PSKA=", AllowedIPs: []string{"10.8.0.2/32"}},
			{PublicKey: "PEERB=", AllowedIPs: []string{"10.8.0.3/32", "192.168.50.0/24"}},
		},
	})
	want := "[Interface]\nPrivateKey = SERVERPRIV=\nAddress = 10.8.0.1/24\nListenPort = 51820\n" +
		"\n[Peer]\nPublicKey = PEERA=\nPresharedKey = PSKA=\nAllowedIPs = 10.8.0.2/32\n" +
		"\n[Peer]\nPublicKey = PEERB=\nAllowedIPs = 10.8.0.3/32, 192.168.50.0/24\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if withMTU := RenderServer(ServerConfig{PrivateKey: "K", Address: "10.8.0.1/24", ListenPort: 1, MTU: 1380}); withMTU !=
		"[Interface]\nPrivateKey = K\nAddress = 10.8.0.1/24\nListenPort = 1\nMTU = 1380\n" {
		t.Fatalf("MTU: %q", withMTU)
	}
}
