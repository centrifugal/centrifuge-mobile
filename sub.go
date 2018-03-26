package centrifuge

import (
	"sync"
	"time"
)

// SubscribeSuccessContext is a subscribe success event context passed to event callback.
type SubscribeSuccessContext struct {
	Resubscribed bool
	Recovered    bool
}

// SubscribeErrorContext is a subscribe error event context passed to event callback.
type SubscribeErrorContext struct {
	Error string
}

// UnsubscribeContext is a context passed to unsubscribe event callback.
type UnsubscribeContext struct{}

// MessageHandler is a function to handle messages in channels.
type MessageHandler interface {
	OnMessage(*Sub, *Message)
}

// JoinHandler is a function to handle join messages.
type JoinHandler interface {
	OnJoin(*Sub, *ClientInfo)
}

// LeaveHandler is a function to handle leave messages.
type LeaveHandler interface {
	OnLeave(*Sub, *ClientInfo)
}

// UnsubscribeHandler is a function to handle unsubscribe event.
type UnsubscribeHandler interface {
	OnUnsubscribe(*Sub, *UnsubscribeContext)
}

// SubscribeSuccessHandler is a function to handle subscribe success event.
type SubscribeSuccessHandler interface {
	OnSubscribeSuccess(*Sub, *SubscribeSuccessContext)
}

// SubscribeErrorHandler is a function to handle subscribe error event.
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

// NewSubEventHandler initializes new SubEventHandler.
func NewSubEventHandler() *SubEventHandler {
	return &SubEventHandler{}
}

// OnMessage allows to set MessageHandler to SubEventHandler.
func (h *SubEventHandler) OnMessage(handler MessageHandler) {
	h.onMessage = handler
}

// OnJoin allows to set JoinHandler to SubEventHandler.
func (h *SubEventHandler) OnJoin(handler JoinHandler) {
	h.onJoin = handler
}

// OnLeave allows to set LeaveHandler to SubEventHandler.
func (h *SubEventHandler) OnLeave(handler LeaveHandler) {
	h.onLeave = handler
}

// OnUnsubscribe allows to set UnsubscribeHandler to SubEventHandler.
func (h *SubEventHandler) OnUnsubscribe(handler UnsubscribeHandler) {
	h.onUnsubscribe = handler
}

// OnSubscribeSuccess allows to set SubscribeSuccessHandler to SubEventHandler.
func (h *SubEventHandler) OnSubscribeSuccess(handler SubscribeSuccessHandler) {
	h.onSubscribeSuccess = handler
}

// OnSubscribeError allows to set SubscribeErrorHandler to SubEventHandler.
func (h *SubEventHandler) OnSubscribeError(handler SubscribeErrorHandler) {
	h.onSubscribeError = handler
}

const (
	NEW = iota
	SUBSCRIBING
	SUBSCRIBED
	SUBERROR
	UNSUBSCRIBED
)

// Sub describes client subscription to channel.
type Sub struct {
	mu              sync.Mutex
	channel         string
	centrifuge      *Client
	status          int
	events          *SubEventHandler
	lastMessageID   *string
	lastMessageMu   sync.RWMutex
	resubscribed    bool
	recovered       bool
	err             error
	needResubscribe bool
	subFutures      []chan error
}

func (c *Client) newSub(channel string, events *SubEventHandler) *Sub {
	s := &Sub{
		centrifuge:      c,
		channel:         channel,
		events:          events,
		subFutures:      make([]chan error, 0),
		needResubscribe: true,
	}
	return s
}

// Channel returns subscription channel.
func (s *Sub) Channel() string {
	return s.channel
}

func (s *Sub) newSubFuture() chan error {
	fut := make(chan error, 1)
	s.mu.Lock()
	if s.status == SUBSCRIBED {
		fut <- nil
	} else if s.status == SUBERROR {
		fut <- s.err
	} else {
		s.subFutures = append(s.subFutures, fut)
	}
	s.mu.Unlock()
	return fut
}

// Sub.mu lock must be held outside.
func (s *Sub) resolveSubFutures(err error) {
	for _, ch := range s.subFutures {
		select {
		case ch <- nil:
		default:
		}
	}
	s.subFutures = nil
}

func (s *Sub) removeSubFuture(subFuture chan error) {
	s.mu.Lock()
	for i, v := range s.subFutures {
		if v == subFuture {
			s.subFutures = append(s.subFutures[:i], s.subFutures[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
}

// Publish allows to publish JSON encoded data to subscription channel.
func (s *Sub) Publish(data []byte) error {
	subFuture := s.newSubFuture()
	select {
	case err := <-subFuture:
		if err != nil {
			return err
		}
		return s.centrifuge.publish(s.channel, data)
	case <-time.After(time.Duration(s.centrifuge.config.TimeoutMilliseconds) * time.Millisecond):
		s.removeSubFuture(subFuture)
		return ErrTimeout
	}
}

func (s *Sub) history() ([]Message, error) {
	subFuture := s.newSubFuture()
	select {
	case err := <-subFuture:
		if err != nil {
			return nil, err
		}
		return s.centrifuge.history(s.channel)
	case <-time.After(time.Duration(s.centrifuge.config.TimeoutMilliseconds) * time.Millisecond):
		s.removeSubFuture(subFuture)
		return nil, ErrTimeout
	}
}

func (s *Sub) presence() (map[string]ClientInfo, error) {
	subFuture := s.newSubFuture()
	select {
	case err := <-subFuture:
		if err != nil {
			return nil, err
		}
		return s.centrifuge.presence(s.channel)
	case <-time.After(time.Duration(s.centrifuge.config.TimeoutMilliseconds) * time.Millisecond):
		s.removeSubFuture(subFuture)
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
	s.mu.Unlock()
	return s.resubscribe()
}

func (s *Sub) triggerOnUnsubscribe(needResubscribe bool) {
	s.mu.Lock()
	if s.status != SUBSCRIBED {
		s.mu.Unlock()
		return
	}
	s.needResubscribe = needResubscribe
	s.status = UNSUBSCRIBED
	s.mu.Unlock()
	if s.events != nil && s.events.onUnsubscribe != nil {
		handler := s.events.onUnsubscribe
		handler.OnUnsubscribe(s, &UnsubscribeContext{})
	}
}

func (s *Sub) subscribeSuccess(recovered bool) {
	s.mu.Lock()
	if s.status == SUBSCRIBED {
		s.mu.Unlock()
		return
	}
	s.status = SUBSCRIBED
	resubscribed := s.resubscribed
	s.resolveSubFutures(nil)
	s.mu.Unlock()
	if s.events != nil && s.events.onSubscribeSuccess != nil {
		handler := s.events.onSubscribeSuccess
		handler.OnSubscribeSuccess(s, &SubscribeSuccessContext{Resubscribed: resubscribed, Recovered: recovered})
	}
	s.mu.Lock()
	s.resubscribed = true
	s.mu.Unlock()
}

func (s *Sub) subscribeError(err error) {
	s.mu.Lock()
	if s.status == SUBERROR {
		s.mu.Unlock()
		return
	}
	s.err = err
	s.status = SUBERROR
	s.resolveSubFutures(err)
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
	if s.status == SUBSCRIBED || s.status == SUBSCRIBING {
		s.mu.Unlock()
		return nil
	}
	needResubscribe := s.needResubscribe
	s.mu.Unlock()
	if !needResubscribe {
		return nil
	}

	s.centrifuge.mutex.Lock()
	if s.centrifuge.status != CONNECTED {
		s.centrifuge.mutex.Unlock()
		return nil
	}
	s.centrifuge.mutex.Unlock()

	s.mu.Lock()
	s.status = SUBSCRIBING
	s.mu.Unlock()

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

	s.subscribeSuccess(body.Recovered)

	return nil
}
