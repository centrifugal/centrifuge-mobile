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

func extractDisconnectWebsocket(err error) *disconnect {
	if err != nil {
		if closeErr, ok := err.(*websocket.CloseError); ok {
			var disconnect disconnect
			err := json.Unmarshal([]byte(closeErr.Text), &disconnect)
			if err == nil {
				return &disconnect
			}
		}
	}
	return nil
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
	closeCh        chan struct{}
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
		replyCh: make(chan *proto.Reply, 128),
		config:  config,
		closeCh: make(chan struct{}),
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
	close(t.closeCh)
	return t.conn.Close()
}

func (t *websocketTransport) reader() {
	defer t.Close()
	defer close(t.replyCh)

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
				case <-t.closeCh:
					return
				case t.replyCh <- reply:
				default:
					// Can't keep up with server message rate.
					t.disconnect = &disconnect{Reason: "client slow", Reconnect: true}
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
