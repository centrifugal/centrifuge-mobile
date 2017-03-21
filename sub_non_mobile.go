// +build !mobile

package centrifuge

// History allows to extract channel history.
func (s *Sub) History() ([]Message, error) {
	return s.history()
}

// Presence allows to extract channel history.
func (s *Sub) Presence() (map[string]ClientInfo, error) {
	return s.presence()
}
