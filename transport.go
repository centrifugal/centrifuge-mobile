package centrifuge

import "github.com/centrifugal/centrifuge-mobile/internal/proto"

type transport interface {
	Read() (*proto.Reply, *disconnect, error)
	Write(*proto.Command) error
	Close() error
}
