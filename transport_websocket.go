package centrifuge

import (
	"fmt"
	"net/http"
	"time"

	"github.com/centrifugal/centrifuge-mobile/internal/proto"
	"github.com/gorilla/websocket"
)

// closeErr tries to extract connection close code and reason from error.
// It returns true as first return value in case of successful extraction.
func closeErr(err error) (bool, int, string) {
	if closeErr, ok := err.(*websocket.CloseError); ok {
		return true, closeErr.Code, closeErr.Text
	}
	return false, 0, ""
}

type websocketTransport struct {
	conn           *websocket.Conn
	commandEncoder proto.CommandEncoder
	replyDecoder   proto.ReplyDecoder
}

type websocketTransportConfig struct {
	encoding     proto.Encoding
	writeTimeout time.Duration
	compression  bool
}

func newWebsocketTransport(url string, config websocketTransport) (transport, error) {
	wsHeaders := http.Header{}
	dialer := websocket.DefaultDialer
	if config.compression {
		dialer.EnableCompression = true
	}
	conn, resp, err := dialer.Dial(url, wsHeaders)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		return nil, fmt.Errorf("Wrong status code while connecting to server: '%d'", resp.StatusCode)
	}
	return &wsTransport{conn: conn, config: config}, nil
}

func (c *wsTransport) Close() error {
	return c.conn.Close()
}

func (c *wsTransport) WriteMessage(msg []byte) error {
	c.conn.SetWriteDeadline(time.Now().Add(c.config.writeTimeout))
	err := c.conn.WriteMessage(websocket.TextMessage, msg)
	c.conn.SetWriteDeadline(time.Time{})
	return err
}

func (c *wsTransport) ReadMessage() ([]byte, error) {
	_, message, err := c.conn.ReadMessage()
	return message, err
}
