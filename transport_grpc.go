package centrifuge

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"sync"

	"github.com/centrifugal/centrifuge-mobile/internal/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
)

func extractDisconnectGRPC(md metadata.MD) *disconnect {
	if value, ok := md["disconnect"]; ok {
		if len(value) > 0 {
			d := value[0]
			var dis disconnect
			err := json.Unmarshal([]byte(d), &dis)
			if err == nil {
				return &dis
			}
		}
	}
	return &disconnect{
		Reason:    "connection closed",
		Reconnect: true,
	}
}

type grpcTransport struct {
	mu         sync.Mutex
	conn       *grpc.ClientConn
	config     grpcTransportConfig
	disconnect *disconnect
	replyCh    chan *proto.Reply
	stream     proto.Centrifuge_CommunicateClient
	closed     bool
}

type grpcTransportConfig struct {
	tls      bool
	certFile string
}

func newGRPCTransport(u string, config grpcTransportConfig) (transport, error) {
	var opts []grpc.DialOption
	if config.tls && config.certFile != "" {
		// openssl req -x509 -nodes -days 365 -newkey rsa:2048 -keyout ./server.key -out ./server.cert
		cred, err := credentials.NewClientTLSFromFile(config.certFile, "")
		if err != nil {
			log.Fatal(err)
		}
		opts = append(opts, grpc.WithTransportCredentials(cred))
	} else if config.tls {
		creds := credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})
		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithInsecure())
	}

	urlObject, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.Dial(urlObject.Host, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %s", err)
	}

	t := &grpcTransport{
		conn:    conn,
		config:  config,
		replyCh: make(chan *proto.Reply, 64),
	}

	client := proto.NewCentrifugeClient(conn)

	stream, err := client.Communicate(context.Background())
	if err != nil {
		return nil, err
	}

	t.stream = stream

	go t.reader()
	return t, nil
}

func (t grpcTransport) Write(cmd *proto.Command) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stream.Send(cmd)
}

func (t *grpcTransport) Read() (*proto.Reply, error) {
	select {
	case reply, ok := <-t.replyCh:
		if !ok {
			return nil, io.EOF
		}
		return reply, nil
	}
}

func (t *grpcTransport) GetDisconnect() *disconnect {
	return t.disconnect
}

func (t *grpcTransport) reader() {
	defer t.Close()

	for {
		reply, err := t.stream.Recv()
		if err != nil {
			disconnect := extractDisconnectGRPC(t.stream.Trailer())
			t.disconnect = disconnect
			return
		}
		select {
		case t.replyCh <- reply:
		default:
			// Can't keep up with
			return
		}
	}
}

func (t *grpcTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	close(t.replyCh)
	return t.conn.Close()
}
