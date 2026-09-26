package relay

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

// DialFunc returns a pgconn.DialFunc that tunnels TCP connections through a DataCluster relay.
// relayBaseURL: base URL of the remote DataCluster instance (e.g. https://dc.example.com).
// token: the relay server's Bearer token (derived from its APP_PASSWORD).
func DialFunc(relayBaseURL, token string) pgconn.DialFunc {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return dial(ctx, relayBaseURL, token, addr)
	}
}

func dial(ctx context.Context, relayBaseURL, token, target string) (net.Conn, error) {
	u, err := url.Parse(strings.TrimRight(relayBaseURL, "/"))
	if err != nil {
		return nil, fmt.Errorf("relay: invalid url: %w", err)
	}

	host := u.Host
	if !strings.Contains(host, ":") {
		if u.Scheme == "https" {
			host += ":443"
		} else {
			host += ":80"
		}
	}

	var rawConn net.Conn
	if u.Scheme == "https" {
		d := &tls.Dialer{Config: &tls.Config{
			ServerName: u.Hostname(),
			NextProtos: []string{"http/1.1"}, // force HTTP/1.1 — HTTP/2 breaks the Upgrade mechanism
		}}
		rawConn, err = d.DialContext(ctx, "tcp", host)
	} else {
		d := &net.Dialer{}
		rawConn, err = d.DialContext(ctx, "tcp", host)
	}
	if err != nil {
		return nil, fmt.Errorf("relay: connect to relay server: %w", err)
	}

	targetHost, targetPort, err := net.SplitHostPort(target)
	if err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("relay: invalid target %q: %w", target, err)
	}

	req := fmt.Sprintf(
		"GET /api/relay?host=%s&port=%s HTTP/1.1\r\nHost: %s\r\nAuthorization: Bearer %s\r\nConnection: Upgrade\r\nUpgrade: tcp-tunnel\r\n\r\n",
		url.QueryEscape(targetHost), url.QueryEscape(targetPort), u.Host, token,
	)
	if _, err = rawConn.Write([]byte(req)); err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("relay: send request: %w", err)
	}

	br := bufio.NewReader(rawConn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		rawConn.Close()
		return nil, fmt.Errorf("relay: read response: %w", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		rawConn.Close()
		return nil, fmt.Errorf("relay: server returned %d (check relay URL and token)", resp.StatusCode)
	}

	if br.Buffered() > 0 {
		return &bufferedConn{Conn: rawConn, br: br}, nil
	}
	return rawConn, nil
}

// bufferedConn wraps net.Conn with a pre-read bufio.Reader to drain any bytes
// the HTTP response reader may have pulled ahead from the wire.
type bufferedConn struct {
	net.Conn
	br *bufio.Reader
}

func (b *bufferedConn) Read(p []byte) (int, error) {
	return b.br.Read(p)
}
