package centrifuge

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
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

func extractDisconnectWebsocket(err error) *disconnect {
	d := &disconnect{
		Reason:    "connection closed",
		Reconnect: true,
	}
	if err != nil {
		ok, _, reason := closeErr(err)
		if ok {
			var disconnectAdvice disconnect
			err := json.Unmarshal([]byte(reason), &disconnectAdvice)
			if err == nil {
				d.Reason = disconnectAdvice.Reason
				d.Reconnect = disconnectAdvice.Reconnect
			}
		}
	}
	return d
}

type websocketTransport struct {
	mu             sync.Mutex
	conn           *websocket.Conn
	commandEncoder proto.CommandEncoder
	replyDecoder   proto.ReplyDecoder
	replyCh        chan *proto.Reply
	config         websocketTransportConfig
	disconnect     *disconnect
	closed         bool
}

type websocketTransportConfig struct {
	encoding     proto.Encoding
	writeTimeout time.Duration
	compression  bool
}

func newWebsocketTransport(url string, config websocketTransportConfig) (transport, error) {
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

	t := &websocketTransport{
		conn:    conn,
		replyCh: make(chan *proto.Reply, 64),
		config:  config,
	}
	go t.reader()
	return t, nil
}

func (t *websocketTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	close(t.replyCh)
	return t.conn.Close()
}

func (t *websocketTransport) reader() {
	defer t.Close()

	for {
		_, data, err := t.conn.ReadMessage()
		if err != nil {
			disconnect := extractDisconnectWebsocket(err)
			t.disconnect = disconnect
			return
		}
	loop:
		for {
			decoder := proto.GetReplyDecoder(t.config.encoding, data)
			for {
				reply, err := decoder.Decode()
				if err != nil {
					if err == io.EOF {
						break loop
					}
					return
				}
				select {
				case t.replyCh <- reply:
				default:
					// Can't keep up with message stream from server.
					return
				}
			}
		}
	}
}

func (t *websocketTransport) Write(cmd *proto.Command) error {
	encoder := proto.GetCommandEncoder(t.config.encoding)
	data, err := encoder.Encode(cmd)
	if err != nil {
		return err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.conn.SetWriteDeadline(time.Now().Add(t.config.writeTimeout))
	if t.config.encoding == proto.EncodingJSON {
		err = t.conn.WriteMessage(websocket.TextMessage, data)
	} else {
		err = t.conn.WriteMessage(websocket.BinaryMessage, data)
	}
	t.conn.SetWriteDeadline(time.Time{})
	return err
}

func (t *websocketTransport) Read() (*proto.Reply, *disconnect, error) {
	reply, ok := <-t.replyCh
	if !ok {
		return nil, t.disconnect, io.EOF
	}
	return reply, nil, nil
}
