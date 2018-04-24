package centrifuge

import "github.com/centrifugal/centrifuge-mobile/internal/proto"

// Error represents client reply error.
type Error = proto.Error

// Raw represents raw bytes.
type Raw = proto.Raw

// Pub allows to deliver custom payload to all channel subscribers.
type Pub = proto.Pub

// ClientInfo is short information about client connection.
type ClientInfo = proto.ClientInfo
