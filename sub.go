package centrifuge

import (
	"sync"
)

// MessageHandler is a function to handle messages in channels.
type MessageHandler interface {
	OnMessage(*Sub, *Message) error
}

// JoinHandler is a function to handle join messages.
type JoinHandler interface {
	OnJoin(*Sub, *ClientInfo) error
}

// LeaveHandler is a function to handle leave messages.
type LeaveHandler interface {
	OnLeave(*Sub, *ClientInfo) error
}

// UnsubscribeHandler is a function to handle unsubscribe event.
type UnsubscribeHandler interface {
	OnUnsubscribe(*Sub) error
}

type SubscribeSuccessHandler interface {
	OnSubscribeSuccess(*Sub)
}

type SubscribeErrorHandler interface {
	OnSubscribeError(*Sub, error)
}

// SubEventHandler contains callback functions that will be called when
// corresponding event happens with subscription to channel.
type SubEventHandler struct {
	onMessage          MessageHandler
	onJoin             JoinHandler
	onLeave            LeaveHandler
	onUnsubscribe      UnsubscribeHandler
	onSubscribeSuccess SubscribeSuccessHandler
	onSubscribeError   SubscribeErrorHandler
}

func NewSubEventHandler() *SubEventHandler {
	return &SubEventHandler{}
}

func (h *SubEventHandler) OnMessage(handler MessageHandler) {
	h.onMessage = handler
}

func (h *SubEventHandler) OnJoin(handler JoinHandler) {
	h.onJoin = handler
}

func (h *SubEventHandler) OnLeave(handler LeaveHandler) {
	h.onLeave = handler
}

func (h *SubEventHandler) OnUnsubscribe(handler UnsubscribeHandler) {
	h.onUnsubscribe = handler
}

func (h *SubEventHandler) OnSubscribeSuccess(handler SubscribeSuccessHandler) {
	h.onSubscribeSuccess = handler
}

func (h *SubEventHandler) OnSubscribeError(handler SubscribeErrorHandler) {
	h.onSubscribeError = handler
}

type Sub struct {
	channel       string
	centrifuge    *Client
	events        *SubEventHandler
	lastMessageID *string
	lastMessageMu sync.RWMutex
}

func (c *Client) newSub(channel string, events *SubEventHandler) *Sub {
	return &Sub{
		centrifuge: c,
		channel:    channel,
		events:     events,
	}
}

func (s *Sub) Channel() string {
	return s.channel
}

// Publish JSON encoded data.
func (s *Sub) Publish(data []byte) error {
	return s.centrifuge.publish(s.channel, data)
}

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
	messages, err := s.centrifuge.history(s.channel)
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
	presence, err := s.centrifuge.presence(s.channel)
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

// Unsubscribe allows to unsubscribe from channel.
func (s *Sub) Unsubscribe() error {
	return s.centrifuge.unsubscribe(s.channel)
}

func (s *Sub) handleMessage(m *Message) {
	var handler MessageHandler
	if s.events != nil && s.events.onMessage != nil {
		handler = s.events.onMessage
	}
	mid := m.UID
	s.lastMessageMu.Lock()
	s.lastMessageID = &mid
	s.lastMessageMu.Unlock()
	if handler != nil {
		handler.OnMessage(s, m)
	}
}

func (s *Sub) handleJoinMessage(info *ClientInfo) {
	var handler JoinHandler
	if s.events != nil && s.events.onJoin != nil {
		handler = s.events.onJoin
	}
	if handler != nil {
		handler.OnJoin(s, info)
	}
}

func (s *Sub) handleLeaveMessage(info *ClientInfo) {
	var handler LeaveHandler
	if s.events != nil && s.events.onLeave != nil {
		handler = s.events.onLeave
	}
	if handler != nil {
		handler.OnLeave(s, info)
	}
}

func (s *Sub) resubscribe() error {
	privateSign, err := s.centrifuge.privateSign(s.channel)
	if err != nil {
		return err
	}
	s.lastMessageMu.Lock()
	msgID := *s.lastMessageID
	s.lastMessageMu.Unlock()
	body, err := s.centrifuge.sendSubscribe(s.channel, &msgID, privateSign)
	if err != nil {
		return err
	}
	if !body.Status {
		return ErrBadSubscribeStatus
	}

	if len(body.Messages) > 0 {
		for i := len(body.Messages) - 1; i >= 0; i-- {
			s.handleMessage(messageFromRaw(&body.Messages[i]))
		}
	} else {
		lastID := string(body.Last)
		s.lastMessageMu.Lock()
		s.lastMessageID = &lastID
		s.lastMessageMu.Unlock()
	}

	// resubscribe successful.
	return nil
}