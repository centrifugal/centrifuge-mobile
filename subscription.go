package centrifuge

import (
	gocentrifuge "github.com/centrifugal/centrifuge-go"
)

// Publication ...
type Publication struct {
	Offset int64
	Data   []byte
	Info   *ClientInfo
}

// ClientInfo ...
type ClientInfo struct {
	Client   string
	User     string
	ConnInfo []byte
	ChanInfo []byte
}

// PresenceStats ...
type PresenceStats struct {
	NumClients int
	NumUsers   int
}

// Subscription describes client subscription to channel.
type Subscription struct {
	sub *gocentrifuge.Subscription
}

// Channel returns subscription channel.
func (s *Subscription) Channel() string {
	return s.sub.Channel()
}

type PublishResult struct{}

// Publish allows to publish JSON encoded data to subscription channel.
func (s *Subscription) Publish(data []byte) (*PublishResult, error) {
	_, err := s.sub.Publish(data)
	if err != nil {
		return nil, err
	}
	return &PublishResult{}, nil
}

// Unsubscribe allows to unsubscribe from channel.
func (s *Subscription) Unsubscribe() error {
	return s.sub.Unsubscribe()
}

// Subscribe allows to subscribe again after unsubscribing.
func (s *Subscription) Subscribe() error {
	return s.sub.Subscribe()
}

// HistoryData ...
type HistoryData struct {
	publications []gocentrifuge.Publication
}

// NumItems to get total number of Publication items in collection.
func (d *HistoryData) NumItems() int {
	return len(d.publications)
}

// ItemAt to get Publication by index.
func (d *HistoryData) ItemAt(i int) *Publication {
	pub := d.publications[i]
	var info *ClientInfo
	if pub.Info != nil {
		info = &ClientInfo{
			Client:   pub.Info.Client,
			User:     pub.Info.User,
			ConnInfo: pub.Info.ConnInfo,
			ChanInfo: pub.Info.ChanInfo,
		}
	}
	return &Publication{
		Offset: int64(pub.Offset),
		Data:   pub.Data,
		Info:   info,
	}
}

// History allows to extract channel history.
func (s *Subscription) History() (*HistoryData, error) {
	res, err := s.sub.History()
	if err != nil {
		return nil, err
	}
	return &HistoryData{
		publications: res.Publications,
	}, nil
}

// PresenceData contains presence information for channel.
type PresenceData struct {
	clients []gocentrifuge.ClientInfo
}

// NumItems to get total number of ClientInfo items in collection.
func (d *PresenceData) NumItems() int {
	return len(d.clients)
}

// ItemAt to get ClientInfo by index.
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
func (s *Subscription) Presence() (*PresenceData, error) {
	res, err := s.sub.Presence()
	if err != nil {
		return nil, err
	}
	clients := make([]gocentrifuge.ClientInfo, len(res.Presence))
	i := 0
	for _, info := range res.Presence {
		clients[i] = info
		i++
	}
	return &PresenceData{
		clients: clients,
	}, nil
}

// PresenceStats allows to extract presence stats information for channel.
func (s *Subscription) PresenceStats() (*PresenceStats, error) {
	stats, err := s.sub.PresenceStats()
	if err != nil {
		return nil, err
	}
	return &PresenceStats{
		NumClients: stats.NumClients,
		NumUsers:   stats.NumUsers,
	}, nil
}
