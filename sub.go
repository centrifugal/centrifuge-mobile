package centrifuge

import (
	gocentrifuge "github.com/centrifugal/centrifuge-go"
)

// Publication ...
type Publication struct {
	UID  string
	Data []byte
	Info *ClientInfo
}

// ClientInfo ...
type ClientInfo struct {
	Client   string
	User     string
	ConnInfo []byte
	ChanInfo []byte
}

// Sub describes client subscription to channel.
type Sub struct {
	sub *gocentrifuge.Sub
}

// Channel returns subscription channel.
func (s *Sub) Channel() string {
	return s.sub.Channel()
}

// Publish allows to publish JSON encoded data to subscription channel.
func (s *Sub) Publish(data []byte) error {
	return s.sub.Publish(data)
}

// Unsubscribe allows to unsubscribe from channel.
func (s *Sub) Unsubscribe() error {
	return s.sub.Unsubscribe()
}

// Subscribe allows to subscribe again after unsubscribing.
func (s *Sub) Subscribe() error {
	return s.sub.Subscribe()
}

// HistoryData ...
type HistoryData struct {
	publications []gocentrifuge.Publication
}

// NumItems ...
func (d *HistoryData) NumItems() int {
	return len(d.publications)
}

// ItemAt ...
func (d *HistoryData) ItemAt(i int) *Publication {
	pub := d.publications[i]
	var info *ClientInfo
	if pub.Info != nil {
		info.Client = pub.Info.Client
		info.User = pub.Info.User
		info.ConnInfo = pub.Info.ConnInfo
		info.ChanInfo = pub.Info.ChanInfo
	}
	return &Publication{
		UID:  pub.UID,
		Data: pub.Data,
		Info: info,
	}
}

// History allows to extract channel history.
func (s *Sub) History() (*HistoryData, error) {
	publications, err := s.sub.History()
	if err != nil {
		return nil, err
	}
	return &HistoryData{
		publications: publications,
	}, nil
}

// PresenceData ...
type PresenceData struct {
	clients []gocentrifuge.ClientInfo
}

// NumItems ...
func (d *PresenceData) NumItems() int {
	return len(d.clients)
}

// ItemAt ...
func (d *PresenceData) ItemAt(i int) *ClientInfo {
	info := d.clients[i]
	return &ClientInfo{
		Client:   info.Client,
		User:     info.User,
		ConnInfo: info.ConnInfo,
		ChanInfo: info.ChanInfo,
	}
}

// Presence allows to extract presence information for channel.
func (s *Sub) Presence() (*PresenceData, error) {
	presence, err := s.sub.Presence()
	if err != nil {
		return nil, err
	}
	clients := make([]gocentrifuge.ClientInfo, len(presence))
	i := 0
	for _, info := range presence {
		clients[i] = info
		i++
	}
	return &PresenceData{
		clients: clients,
	}, nil
}
