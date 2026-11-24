// Package gamequery asks the local game engine whether it answers Source query
// protocol requests. A valid reply proves the game accepts queries on the
// registered port, which the service being active alone does not prove.
package gamequery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

var timeout = time.Second

const infoRequest = "\xff\xff\xff\xffTSource Engine Query\x00"

type ServerInfo struct {
	Map        string
	Players    int
	MaxPlayers int
}

// Query sends an A2S_INFO query to the loopback game port and returns the
// reported map and player counts.
func Query(ctx context.Context, port int) (result ServerInfo, err error) {
	if port < 1 || port > 65535 {
		return result, errors.New("no registered game port to query")
	}
	conn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		return result, err
	}
	defer conn.Close()
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err = conn.SetDeadline(deadline); err != nil {
		return result, err
	}
	request := []byte(infoRequest)
	reply, err := roundTrip(conn, request)
	if err != nil {
		return result, err
	}
	// Several engines answer the first request with a challenge instead of info.
	if isChallenge(reply) {
		reply, err = roundTrip(conn, append(request, reply[5:9]...))
		if err != nil {
			return result, err
		}
		if isChallenge(reply) {
			return result, errors.New("game query challenge was not resolved")
		}
	}
	return parseInfo(reply)
}

// Info retains the player-count interface for callers that do not need a map.
func Info(ctx context.Context, port int) (players, maxPlayers int, err error) {
	result, err := Query(ctx, port)
	return result.Players, result.MaxPlayers, err
}

func roundTrip(conn *net.UDPConn, payload []byte) ([]byte, error) {
	if _, err := conn.Write(payload); err != nil {
		return nil, err
	}
	reply := make([]byte, 1500)
	n, err := conn.Read(reply)
	if err != nil {
		return nil, err
	}
	return reply[:n], nil
}

func isChallenge(reply []byte) bool {
	return len(reply) >= 9 && reply[0] == 0xFF && reply[1] == 0xFF && reply[2] == 0xFF && reply[3] == 0xFF && reply[4] == 0x41
}

func parseInfo(reply []byte) (ServerInfo, error) {
	var result ServerInfo
	if len(reply) < 5 || reply[0] != 0xFF || reply[1] != 0xFF || reply[2] != 0xFF || reply[3] != 0xFF {
		return result, fmt.Errorf("unrecognized game query reply: %x", head(reply))
	}
	r := reader{data: reply[4:]}
	switch reply[4] {
	case 0x49: // Modern info: protocol, name, map, folder, game, id, players...
		r.skip(2) // type and protocol
		r.string()
		result.Map = r.string()
		r.string()
		r.string()
		r.skip(2)
	case 0x6D: // Obsolete info: address, name, map, folder, game, players...
		r.skip(1) // type
		r.string()
		r.string()
		result.Map = r.string()
		r.string()
		r.string()
	default:
		return result, fmt.Errorf("unsupported game query reply: %x", head(reply))
	}
	result.Players, result.MaxPlayers = int(r.byte()), int(r.byte())
	if r.err != nil {
		return ServerInfo{}, fmt.Errorf("truncated game query reply: %x", head(reply))
	}
	return result, nil
}

func head(reply []byte) []byte {
	if len(reply) > 32 {
		return reply[:32]
	}
	return reply
}

type reader struct {
	data []byte
	err  error
}

func (r *reader) byte() byte {
	if len(r.data) < 1 {
		r.err = errors.New("truncated game query reply")
		return 0
	}
	value := r.data[0]
	r.data = r.data[1:]
	return value
}
func (r *reader) skip(count int) {
	if len(r.data) < count {
		r.err = errors.New("truncated game query reply")
		r.data = nil
		return
	}
	r.data = r.data[count:]
}
func (r *reader) string() string {
	for i, b := range r.data {
		if b == 0 {
			value := string(r.data[:i])
			r.data = r.data[i+1:]
			return value
		}
	}
	r.err = errors.New("truncated game query reply")
	r.data = nil
	return ""
}
