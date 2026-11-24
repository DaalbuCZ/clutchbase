package gamequery

import (
	"context"
	"net"
	"os"
	"strconv"
	"testing"
	"time"
)

// serveA2S answers loopback queries with canned responses until the test ends.
func serveA2S(t *testing.T, port int, respond func([]byte) []byte) *net.UDPConn {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			if reply := respond(buf[:n]); reply != nil {
				conn.WriteToUDP(reply, from)
			}
		}
	}()
	return conn
}

func infoReply(players, max byte) []byte {
	reply := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x49, 24}
	reply = append(reply, "clutchbase\x00de_mirage\x00csgo\x00CS2\x00"...)
	// Two app-id bytes precede the player counts in the modern reply.
	return append(reply, 0, 0, players, max, 0, 'd', 'l', 0, 0)
}

// TestRealServerQuery verifies the parser against a live CS2 server instead of
// canned replies. Set CS2_PROBE_PORT to the game port of a running server.
func TestRealServerQuery(t *testing.T) {
	value := os.Getenv("CS2_PROBE_PORT")
	if value == "" {
		t.Skip("set CS2_PROBE_PORT to a running CS2 game port to verify real engine replies")
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		t.Fatalf("invalid CS2_PROBE_PORT %q", value)
	}
	players, max, err := Info(context.Background(), port)
	if err != nil {
		t.Fatalf("real server query failed: %v", err)
	}
	t.Logf("real CS2 answered: %d/%d players", players, max)
}

func TestInfoParsesModernAndObsoleteReplies(t *testing.T) {
	var port int
	if conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)}); err != nil {
		t.Fatal(err)
	} else {
		port = conn.LocalAddr().(*net.UDPAddr).Port
		conn.Close()
	}
	serveA2S(t, port, func([]byte) []byte { return infoReply(3, 12) })
	players, max, err := Info(context.Background(), port)
	if err != nil || players != 3 || max != 12 {
		t.Fatalf("modern reply not parsed: %d/%d %v", players, max, err)
	}
	observed, err := Query(context.Background(), port)
	if err != nil || observed.Map != "de_mirage" || observed.Players != 3 || observed.MaxPlayers != 12 {
		t.Fatalf("modern map not parsed: %+v %v", observed, err)
	}

	obsolete := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x6D}
	obsolete = append(obsolete, "1.2.3.4:27015\x00name\x00de_nuke\x00csgo\x00CS2\x00"...)
	obsolete = append(obsolete, 2, 12, 24)
	serveA2S(t, port+1, func([]byte) []byte { return obsolete })
	players, max, err = Info(context.Background(), port+1)
	if err != nil || players != 2 || max != 12 {
		t.Fatalf("obsolete reply not parsed: %d/%d %v", players, max, err)
	}
	observed, err = Query(context.Background(), port+1)
	if err != nil || observed.Map != "de_nuke" || observed.Players != 2 || observed.MaxPlayers != 12 {
		t.Fatalf("obsolete map not parsed: %+v %v", observed, err)
	}
}

func TestInfoResolvesChallenge(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	conn.Close()
	challenge := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x41, 1, 2, 3, 4}
	serveA2S(t, port, func(query []byte) []byte {
		// Attach the info only when the query carries the expected challenge.
		if len(query) >= 9 && query[len(query)-4] == 1 && query[len(query)-3] == 2 && query[len(query)-2] == 3 && query[len(query)-1] == 4 {
			return infoReply(1, 10)
		}
		return challenge
	})
	players, max, err := Info(context.Background(), port)
	if err != nil || players != 1 || max != 10 {
		t.Fatalf("challenge flow not resolved: %d/%d %v", players, max, err)
	}
}

func TestInfoRejectsGarbageAndSilence(t *testing.T) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	port := conn.LocalAddr().(*net.UDPAddr).Port
	conn.Close()
	serveA2S(t, port, func([]byte) []byte { return []byte("not a source reply") })
	if _, _, err := Info(context.Background(), port); err == nil {
		t.Fatal("accepted an unparseable reply")
	}
	serveA2S(t, port+1, func([]byte) []byte { return nil })
	saved := timeout
	timeout = 80 * time.Millisecond
	defer func() { timeout = saved }()
	if _, _, err := Info(context.Background(), port+1); err == nil {
		t.Fatal("a silent port must not prove readiness")
	}
	if _, _, err := Info(context.Background(), 0); err == nil {
		t.Fatal("accepted an unregistered port")
	}
}
