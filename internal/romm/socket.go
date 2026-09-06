// Socket.IO subscription to RomM's shortcut queue notifications.
//
// The server emits `shortcuts:changed` to the room a device-bound client token
// joins on connect, carrying only the device id: the payload is a nudge, and
// the queue itself is always read back over REST. Everything here is therefore
// best-effort. A dropped connection or a missed event costs latency, never
// correctness, because the caller also polls.
package romm

import (
	"context"
	"net/url"
	"strings"
	"time"

	socketio "github.com/maldikhan/go.socket.io/socket.io/v5/client"
)

// SocketPath is where RomM mounts its main Socket.IO namespace.
const SocketPath = "/ws/socket.io/"

// socketLogger silences the client's own output; the companion reports
// connection state through its own logging instead.
type socketLogger struct{}

func (socketLogger) Debugf(string, ...any) {}
func (socketLogger) Infof(string, ...any)  {}
func (socketLogger) Warnf(string, ...any)  {}
func (socketLogger) Errorf(string, ...any) {}

// Watch connects to RomM and calls onChange whenever the server says this
// device's queue moved. It blocks until ctx is cancelled, reconnecting on its
// own. The error it returns is ctx.Err() in the normal case.
func (c *Client) Watch(ctx context.Context, onChange func()) error {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return err
	}
	u.Path = SocketPath

	client, err := socketio.NewClient(
		socketio.WithURL(u),
		socketio.WithLogger(socketLogger{}),
	)
	if err != nil {
		return err
	}
	// RomM's connect handler reads the client token from the handshake, the
	// same credential the REST calls carry.
	client.SetHandshakeData(map[string]any{"token": c.Token})
	client.On("shortcuts:changed", func(...any) { onChange() })

	// Fire on every connection too: a queue that moved while the socket was
	// down produces no event, so the reconnect itself is the signal to look.
	if err := client.Connect(ctx, func(any) { onChange() }); err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	<-ctx.Done()
	return ctx.Err()
}

// WatchWithRetry keeps Watch running for the life of ctx, backing off when the
// server is unreachable so a stopped RomM does not become a reconnect storm.
func (c *Client) WatchWithRetry(ctx context.Context, onChange func(), onError func(error)) {
	const (
		minBackoff = 2 * time.Second
		maxBackoff = 2 * time.Minute
	)
	backoff := minBackoff
	for ctx.Err() == nil {
		err := c.Watch(ctx, onChange)
		if ctx.Err() != nil {
			return
		}
		if err != nil && onError != nil {
			onError(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// SocketURL is the endpoint Watch dials, for diagnostics.
func (c *Client) SocketURL() string {
	return strings.TrimRight(c.BaseURL, "/") + SocketPath
}
