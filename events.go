package centrifuge

import (
	gocentrifuge "github.com/centrifugal/centrifuge-go"
)

// PrivateSign confirmes that client can subscribe on private channel.
type PrivateSign struct {
	Sign string
	Info string
}

// PrivateSubEvent contains info required to create PrivateSign when client
// wants to subscribe on private channel.
type PrivateSubEvent struct {
	ClientID string
	Channel  string
}

// ConnectEvent is a connect event context passed to OnConnect callback.
type ConnectEvent struct {
	ClientID string
	Version  string
	Data     []byte
}

// DisconnectEvent is a disconnect event context passed to OnDisconnect callback.
type DisconnectEvent struct {
	Reason    string
	Reconnect bool
}

// ErrorEvent is an error event context passed to OnError callback.
type ErrorEvent struct {
	Message string
}

// MessageEvent is an event for async message from server to client.
type MessageEvent struct {
	Data []byte
}

// ConnectHandler is an interface describing how to handle connect event.
type ConnectHandler interface {
	OnConnect(*Client, *ConnectEvent)
}

// DisconnectHandler is an interface describing how to handle disconnect event.
type DisconnectHandler interface {
	OnDisconnect(*Client, *DisconnectEvent)
}

// MessageHandler is an interface describing how to async message from server.
type MessageHandler interface {
	OnMessage(*Client, *MessageEvent)
}

// PrivateSubHandler is an interface describing how to handle private subscription request.
type PrivateSubHandler interface {
	OnPrivateSub(*Client, *PrivateSubEvent) (*PrivateSign, error)
}

// RefreshHandler is an interface describing how to handle credentials refresh event.
type RefreshHandler interface {
	OnRefresh(*Client) (*Credentials, error)
}

// ErrorHandler is an interface describing how to handle error event.
type ErrorHandler interface {
	OnError(*Client, *ErrorEvent)
}

// EventHandler ...
type EventHandler struct {
	client       *Client
	eventHandler *gocentrifuge.EventHandler
}

// NewEventHandler ...
func NewEventHandler() *EventHandler {
	return &EventHandler{
		eventHandler: gocentrifuge.NewEventHandler(),
	}
}

func (h *EventHandler) setClient(c *Client) {
	h.client = c
}

type eventProxy struct {
	client *Client

	onConnect    ConnectHandler
	onDisconnect DisconnectHandler
	onPrivateSub PrivateSubHandler
	onRefresh    RefreshHandler
	onError      ErrorHandler
	onMessage    MessageHandler
}

func (p *eventProxy) OnConnect(c *gocentrifuge.Client, e gocentrifuge.ConnectEvent) {
	p.onConnect.OnConnect(p.client, &ConnectEvent{
		ClientID: e.ClientID,
		Version:  e.Version,
		Data:     e.Data,
	})
}

func (p *eventProxy) OnDisconnect(c *gocentrifuge.Client, e gocentrifuge.DisconnectEvent) {
	p.onDisconnect.OnDisconnect(p.client, &DisconnectEvent{
		Reason:    e.Reason,
		Reconnect: e.Reconnect,
	})
}

func (p *eventProxy) OnPrivateSub(c *gocentrifuge.Client, e gocentrifuge.PrivateSubEvent) (gocentrifuge.PrivateSign, error) {
	sign, err := p.onPrivateSub.OnPrivateSub(p.client, &PrivateSubEvent{
		ClientID: e.ClientID,
		Channel:  e.Channel,
	})
	if err != nil {
		return gocentrifuge.PrivateSign{}, err
	}
	return gocentrifuge.PrivateSign{
		Sign: sign.Sign,
		Info: sign.Info,
	}, nil
}

func (p *eventProxy) OnRefresh(c *gocentrifuge.Client) (gocentrifuge.Credentials, error) {
	credentials, err := p.onRefresh.OnRefresh(p.client)
	if err != nil {
		return gocentrifuge.Credentials{}, err
	}
	return gocentrifuge.Credentials{
		User: credentials.User,
		Exp:  credentials.Exp,
		Info: credentials.Info,
		Sign: credentials.Sign,
	}, nil
}

func (p *eventProxy) OnError(c *gocentrifuge.Client, e gocentrifuge.ErrorEvent) {
	p.onError.OnError(p.client, &ErrorEvent{
		Message: e.Message,
	})
}

func (p *eventProxy) OnMessage(c *gocentrifuge.Client, e gocentrifuge.MessageEvent) {
	p.onMessage.OnMessage(p.client, &MessageEvent{
		Data: e.Data,
	})
}

// OnConnect is a function to handle connect event.
func (h *EventHandler) OnConnect(handler ConnectHandler) {
	proxy := &eventProxy{client: h.client, onConnect: handler}
	h.eventHandler.OnConnect(proxy)
}

// OnDisconnect is a function to handle disconnect event.
func (h *EventHandler) OnDisconnect(handler DisconnectHandler) {
	proxy := &eventProxy{client: h.client, onDisconnect: handler}
	h.eventHandler.OnDisconnect(proxy)
}

// OnPrivateSub needed to handle private channel subscriptions.
func (h *EventHandler) OnPrivateSub(handler PrivateSubHandler) {
	proxy := &eventProxy{client: h.client, onPrivateSub: handler}
	h.eventHandler.OnPrivateSub(proxy)
}

// OnRefresh handles refresh event when client's credentials expired and must be refreshed.
func (h *EventHandler) OnRefresh(handler RefreshHandler) {
	proxy := &eventProxy{client: h.client, onRefresh: handler}
	h.eventHandler.OnRefresh(proxy)
}

// OnError is a function that will receive unhandled errors for logging.
func (h *EventHandler) OnError(handler ErrorHandler) {
	proxy := &eventProxy{client: h.client, onError: handler}
	h.eventHandler.OnError(proxy)
}

// OnMessage allows to process async message from server to client.
func (h *EventHandler) OnMessage(handler MessageHandler) {
	proxy := &eventProxy{client: h.client, onMessage: handler}
	h.eventHandler.OnMessage(proxy)
}

// SubscribeSuccessEvent is a subscribe success event context passed
// to event callback.
type SubscribeSuccessEvent struct {
	Resubscribed bool
	Recovered    bool
}

// SubscribeErrorEvent is a subscribe error event context passed to
// event callback.
type SubscribeErrorEvent struct {
	Error string
}

// UnsubscribeEvent is an event passed to unsubscribe event handler.
type UnsubscribeEvent struct{}

// LeaveEvent has info about user who left channel.
type LeaveEvent struct {
	Client   string
	User     string
	ConnInfo []byte
	ChanInfo []byte
}

// JoinEvent has info about user who joined channel.
type JoinEvent struct {
	Client   string
	User     string
	ConnInfo []byte
	ChanInfo []byte
}

// PublishEvent has info about received channel Publication.
type PublishEvent struct {
	UID  string
	Data []byte
	Info *ClientInfo
}

// PublishHandler is a function to handle messages published in
// channels.
type PublishHandler interface {
	OnPublish(*Sub, *PublishEvent)
}

// JoinHandler is a function to handle join messages.
type JoinHandler interface {
	OnJoin(*Sub, *JoinEvent)
}

// LeaveHandler is a function to handle leave messages.
type LeaveHandler interface {
	OnLeave(*Sub, *LeaveEvent)
}

// UnsubscribeHandler is a function to handle unsubscribe event.
type UnsubscribeHandler interface {
	OnUnsubscribe(*Sub, *UnsubscribeEvent)
}

// SubscribeSuccessHandler is a function to handle subscribe success
// event.
type SubscribeSuccessHandler interface {
	OnSubscribeSuccess(*Sub, *SubscribeSuccessEvent)
}

// SubscribeErrorHandler is a function to handle subscribe error event.
type SubscribeErrorHandler interface {
	OnSubscribeError(*Sub, *SubscribeErrorEvent)
}

// SubEventHandler ...
type SubEventHandler struct {
	sub             *Sub
	subEventHandler *gocentrifuge.SubEventHandler
}

// NewSubEventHandler initializes new SubEventHandler.
func NewSubEventHandler() *SubEventHandler {
	return &SubEventHandler{
		subEventHandler: gocentrifuge.NewSubEventHandler(),
	}
}

func (h *SubEventHandler) setSub(s *Sub) {
	h.sub = s
}

type subEventProxy struct {
	sub *Sub

	onPublish          PublishHandler
	onJoin             JoinHandler
	onLeave            LeaveHandler
	onUnsubscribe      UnsubscribeHandler
	onSubscribeSuccess SubscribeSuccessHandler
	onSubscribeError   SubscribeErrorHandler
}

func (p *subEventProxy) OnPublish(s *gocentrifuge.Sub, e gocentrifuge.PublishEvent) {
	pub := Publication{
		UID:  e.UID,
		Data: e.Data,
	}
	if e.Info != nil {
		pub.Info = &ClientInfo{
			Client:   e.Info.Client,
			User:     e.Info.User,
			ConnInfo: e.Info.ConnInfo,
			ChanInfo: e.Info.ChanInfo,
		}
	}

	event := &PublishEvent{
		UID:  e.UID,
		Data: e.Data,
	}
	if e.Info != nil {
		event.Info = &ClientInfo{
			Client:   e.Info.Client,
			User:     e.Info.User,
			ConnInfo: e.Info.ConnInfo,
			ChanInfo: e.Info.ChanInfo,
		}
	}

	p.onPublish.OnPublish(p.sub, event)
}

func (p *subEventProxy) OnJoin(s *gocentrifuge.Sub, e gocentrifuge.JoinEvent) {
	p.onJoin.OnJoin(p.sub, &JoinEvent{
		Client:   e.Client,
		User:     e.User,
		ConnInfo: e.ConnInfo,
		ChanInfo: e.ChanInfo,
	})
}

func (p *subEventProxy) OnLeave(s *gocentrifuge.Sub, e gocentrifuge.LeaveEvent) {
	p.onLeave.OnLeave(p.sub, &LeaveEvent{
		Client:   e.Client,
		User:     e.User,
		ConnInfo: e.ConnInfo,
		ChanInfo: e.ChanInfo,
	})
}

func (p *subEventProxy) OnUnsubscribe(s *gocentrifuge.Sub, e gocentrifuge.UnsubscribeEvent) {
	p.onUnsubscribe.OnUnsubscribe(p.sub, &UnsubscribeEvent{})
}

func (p *subEventProxy) OnSubscribeSuccess(s *gocentrifuge.Sub, e gocentrifuge.SubscribeSuccessEvent) {
	p.onSubscribeSuccess.OnSubscribeSuccess(p.sub, &SubscribeSuccessEvent{
		Resubscribed: e.Resubscribed,
		Recovered:    e.Recovered,
	})
}

func (p *subEventProxy) OnSubscribeError(s *gocentrifuge.Sub, e gocentrifuge.SubscribeErrorEvent) {
	p.onSubscribeError.OnSubscribeError(p.sub, &SubscribeErrorEvent{
		Error: e.Error,
	})
}

// OnPublish allows to set PublishHandler to SubEventHandler.
func (h *SubEventHandler) OnPublish(handler PublishHandler) {
	proxy := &subEventProxy{sub: h.sub, onPublish: handler}
	h.subEventHandler.OnPublish(proxy)
}

// OnJoin allows to set JoinHandler to SubEventHandler.
func (h *SubEventHandler) OnJoin(handler JoinHandler) {
	proxy := &subEventProxy{sub: h.sub, onJoin: handler}
	h.subEventHandler.OnPublish(proxy)
}

// OnLeave allows to set LeaveHandler to SubEventHandler.
func (h *SubEventHandler) OnLeave(handler LeaveHandler) {
	proxy := &subEventProxy{sub: h.sub, onLeave: handler}
	h.subEventHandler.OnPublish(proxy)
}

// OnUnsubscribe allows to set UnsubscribeHandler to SubEventHandler.
func (h *SubEventHandler) OnUnsubscribe(handler UnsubscribeHandler) {
	proxy := &subEventProxy{sub: h.sub, onUnsubscribe: handler}
	h.subEventHandler.OnPublish(proxy)
}

// OnSubscribeSuccess allows to set SubscribeSuccessHandler to SubEventHandler.
func (h *SubEventHandler) OnSubscribeSuccess(handler SubscribeSuccessHandler) {
	proxy := &subEventProxy{sub: h.sub, onSubscribeSuccess: handler}
	h.subEventHandler.OnPublish(proxy)
}

// OnSubscribeError allows to set SubscribeErrorHandler to SubEventHandler.
func (h *SubEventHandler) OnSubscribeError(handler SubscribeErrorHandler) {
	proxy := &subEventProxy{sub: h.sub, onSubscribeError: handler}
	h.subEventHandler.OnPublish(proxy)
}
