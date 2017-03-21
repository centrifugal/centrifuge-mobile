// +build mobile

package centrifuge

type HistoryData struct {
	messages []Message
}

func (d *HistoryData) NumMessages() int {
	return len(d.messages)
}

func (d *HistoryData) MessageAt(i int) *Message {
	if i > len(d.messages)-1 {
		return nil
	}
	return &d.messages[i]
}

// History allows to extract channel history.
func (s *Sub) History() (*HistoryData, error) {
	messages, err := s.history()
	if err != nil {
		return nil, err
	}
	return &HistoryData{
		messages: messages,
	}, nil
}

type PresenceData struct {
	clients []ClientInfo
}

func (d *PresenceData) NumClients() int {
	return len(d.clients)
}

func (d *PresenceData) ClientAt(i int) *ClientInfo {
	if i > len(d.clients)-1 {
		return nil
	}
	return &d.clients[i]
}

// Presence allows to extract presence information for channel.
func (s *Sub) Presence() (*PresenceData, error) {
	presence, err := s.presence()
	if err != nil {
		return nil, err
	}
	clients := make([]ClientInfo, len(presence))
	i := 0
	for _, info := range presence {
		clients[i] = info
		i += 1
	}
	return &PresenceData{
		clients: clients,
	}, nil
}
