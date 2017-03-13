package centrifuge

import (
	"sync"
	"time"
)

type SubscribeSuccessContext struct {
	IsResubscribe bool
	Recovered     bool
}

type SubscribeErrorContext struct {
	Error string
}

type UnsubscribeContext struct{}

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
	OnUnsubscribe(*Sub, *UnsubscribeContext)
}

type SubscribeSuccessHandler interface {
	OnSubscribeSuccess(*Sub, *SubscribeSuccessContext)
}

type SubscribeErrorHandler interface {
	OnSubscribeError(*Sub, *SubscribeErrorContext)
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

const (
	SUBSCRIBING = iota
	SUBSCRIBED
	FAILED
	UNSUBSCRIBED
)

type Sub struct {
	mu              sync.Mutex
	channel         string
	centrifuge      *Client
	status          int
	events          *SubEventHandler
	lastMessageID   *string
	lastMessageMu   sync.RWMutex
	isResubscribe   bool
	recovered       bool
	err             error
	subscribeCh     chan struct{}
	errorCh         chan struct{}
	needResubscribe bool
}

func (c *Client) newSub(channel string, events *SubEventHandler) *Sub {
	s := &Sub{
		centrifuge:      c,
		channel:         channel,
		events:          events,
		subscribeCh:     make(chan struct{}),
		errorCh:         make(chan struct{}),
		needResubscribe: true,
	}
	return s
}

func (s *Sub) Channel() string {
	return s.channel
}

// Publish JSON encoded data.
func (s *Sub) Publish(data []byte) error {
	s.mu.Lock()
	subCh := s.subscribeCh
	errCh := s.errorCh
	s.mu.Unlock()
	select {
	case <-subCh:
		return s.centrifuge.publish(s.channel, data)
	case <-errCh:
		s.mu.Lock()
		err := s.err
		s.mu.Unlock()
		return err
	case <-time.After(time.Duration(s.centrifuge.config.TimeoutMilliseconds) * time.Millisecond):
		return ErrTimeout
	}
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
	s.mu.Lock()
	subCh := s.subscribeCh
	errCh := s.errorCh
	s.mu.Unlock()
	select {
	case <-subCh:
		messages, err := s.centrifuge.history(s.channel)
		if err != nil {
			return nil, err
		}
		return &HistoryData{
			messages: messages,
		}, nil
	case <-errCh:
		s.mu.Lock()
		err := s.err
		s.mu.Unlock()
		return nil, err
	case <-time.After(time.Duration(s.centrifuge.config.TimeoutMilliseconds) * time.Millisecond):
		return nil, ErrTimeout
	}
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
	s.mu.Lock()
	subCh := s.subscribeCh
	errCh := s.errorCh
	s.mu.Unlock()
	select {
	case <-subCh:
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
	case <-errCh:
		s.mu.Lock()
		err := s.err
		s.mu.Unlock()
		return nil, err
	case <-time.After(time.Duration(s.centrifuge.config.TimeoutMilliseconds) * time.Millisecond):
		return nil, ErrTimeout
	}
}

// Unsubscribe allows to unsubscribe from channel.
func (s *Sub) Unsubscribe() error {
	s.centrifuge.unsubscribe(s.channel)
	s.triggerOnUnsubscribe(false)
	return nil
}

// Subscribe allows to subscribe again after unsubscribing.
func (s *Sub) Subscribe() error {
	s.mu.Lock()
	s.needResubscribe = true
	s.status = SUBSCRIBING
	s.mu.Unlock()
	return s.resubscribe()
}

func (s *Sub) triggerOnUnsubscribe(needResubscribe bool) {
	s.mu.Lock()
	if s.status == UNSUBSCRIBED {
		s.mu.Unlock()
		return
	}
	s.needResubscribe = needResubscribe
	s.status = UNSUBSCRIBED
	s.subscribeCh = make(chan struct{})
	s.errorCh = make(chan struct{})
	s.mu.Unlock()
	if s.events != nil && s.events.onUnsubscribe != nil {
		handler := s.events.onUnsubscribe
		handler.OnUnsubscribe(s, &UnsubscribeContext{})
	}
}

func (s *Sub) subscribeSuccess() {
	s.mu.Lock()
	if s.status == SUBSCRIBED {
		s.mu.Unlock()
		return
	}
	s.status = SUBSCRIBED
	close(s.subscribeCh)
	s.mu.Unlock()
	if s.events != nil && s.events.onSubscribeSuccess != nil {
		handler := s.events.onSubscribeSuccess
		handler.OnSubscribeSuccess(s, &SubscribeSuccessContext{})
	}
}

func (s *Sub) subscribeError(err error) {
	s.mu.Lock()
	if s.status == FAILED {
		s.mu.Unlock()
		return
	}
	s.err = err
	s.status = FAILED
	close(s.errorCh)
	s.mu.Unlock()
	if s.events != nil && s.events.onSubscribeError != nil {
		handler := s.events.onSubscribeError
		handler.OnSubscribeError(s, &SubscribeErrorContext{Error: err.Error()})
	}
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
	s.mu.Lock()
	needResubscribe := s.needResubscribe
	s.mu.Unlock()
	if !needResubscribe {
		return nil
	}

	privateSign, err := s.centrifuge.privateSign(s.channel)
	if err != nil {
		return err
	}

	var msgID *string
	s.lastMessageMu.Lock()
	if s.lastMessageID != nil {
		msg := *s.lastMessageID
		msgID = &msg
	}
	s.lastMessageMu.Unlock()
	body, err := s.centrifuge.sendSubscribe(s.channel, msgID, privateSign)
	if err != nil {
		s.subscribeError(err)
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

	s.subscribeSuccess()

	return nil
}
