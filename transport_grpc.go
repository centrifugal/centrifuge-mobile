package centrifuge

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	replyCh    chan proto.Reply
}

type grpcTransportConfig struct {
	tls      bool
	certFile string
}

func newGRPCTransport(addr string, config grpcTransportConfig) (transport, error) {
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

	conn, err := grpc.Dial(*addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to dial: %s", err)
	}

	t := &grpcTransport{
		conn:    conn,
		config:  config,
		replyCh: make(chan proto.Reply, 64),
	}

	client := proto.NewCentrifugeClient(conn)

	stream, err := client.Communicate(context.Background())
	if err != nil {
		return nil, err
	}

	go t.reader(stream)
	go t.writer(stream)
	return t, nil
}

func (t grpcTransport) Write(*proto.Reply) error {

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

func (t *grpcTransport) reader(stream proto.Centrifuge_CommunicateClient) {
	decoder := proto.GetReplyDecoder(proto.EncodingProtobuf, data)
	defer proto.PutReplyDecoder(proto.EncodingProtobuf, decoder)
	defer t.Close()

	for {
		data, err := stream.Recv()
		if err != nil {
			disconnect := extractDisconnectGRPC(stream.Trailer())
			t.disconnect = disconnect
			return
		}
		for {
			reply, err := decoder.Decode()
			if err != nil {
				if err == io.EOF {
					break
				}
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

// very naive Centrifugo GRPC client example.
func run() {

	conn, err := grpc.Dial(*addr, opts...)
	if err != nil {
		log.Fatalf("failed to connect: %s", err)
	}
	defer conn.Close()

	cl := proto.NewCentrifugeClient(conn)

	stream, err := cl.Communicate(context.Background())
	if err != nil {
		log.Println(err)
		return
	}

	err = stream.Send(&proto.Command{
		ID:     uint32(nextID()),
		Method: proto.MethodTypeConnect,
	})

	if err != nil {
		log.Println(err)
		return
	}

	rep, err := stream.Recv()

	if err != nil {
		log.Printf("%#v\n", extractDisconnect(stream.Trailer()))
		log.Println(err)
		return
	}

	var connectResult proto.ConnectResult
	err = connectResult.Unmarshal(rep.Result)
	if err != nil {
		log.Println(err)
		return
	}

	log.Printf("%#v\n", connectResult)

	subscribeRequest := &proto.SubscribeRequest{
		Channel: *channel,
	}

	params, _ := subscribeRequest.Marshal()

	err = stream.Send(&proto.Command{
		ID:     uint32(nextID()),
		Method: proto.MethodTypeSubscribe,
		Params: params,
	})

	rep, err = stream.Recv()

	if err != nil {
		log.Printf("%#v\n", extractDisconnect(stream.Trailer()))
		log.Println(err)
		return
	}

	var subscribeResult proto.SubscribeResult
	err = subscribeResult.Unmarshal(rep.Result)
	if err != nil {
		log.Println(err)
		return
	}

	log.Printf("%#v\n", subscribeResult)

	for {
		rep, err := stream.Recv()
		if err != nil {
			log.Printf("%#v\n", extractDisconnect(stream.Trailer()))
			log.Println(err)
			return
		}
		if rep.ID == 0 {
			var message proto.Message
			err = message.Unmarshal(rep.Result)
			if err != nil {
				log.Println(err)
				return
			}
			if message.Type == proto.MessageTypePub {
				var publication proto.Pub
				err = publication.Unmarshal(message.Data)
				if err != nil {
					log.Println(err)
					return
				}
				log.Printf("%#v with data: %s\n", publication, string(publication.Data))
			}
		}
	}
}
