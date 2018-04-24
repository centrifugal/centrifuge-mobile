// +build !mobile

package centrifuge

import "github.com/centrifugal/centrifuge-mobile/internal/proto"

// History allows to extract channel history.
func (s *Sub) History() ([]proto.Pub, error) {
	return s.history()
}

// Presence allows to extract channel history.
func (s *Sub) Presence() (map[string]proto.ClientInfo, error) {
	return s.presence()
}
