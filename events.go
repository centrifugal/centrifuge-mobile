package centrifuge

import (
	"fmt"

	gocentrifuge "github.com/centrifugal/centrifuge-go"
)

// PrivateSign confirmes that client can subscribe on private channel.
type PrivateSign struct {
	Token string
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
	OnPrivateSub(*Client, *PrivateSubEvent) (string, error)
}

// RefreshHandler is an interface describing how to handle credentials refresh event.
type RefreshHandler interface {
	OnRefresh(*Client) (string, error)
}

// ErrorHandler is an interface describing how to handle error event.
type ErrorHandler interface {
	OnError(*Client, *ErrorEvent)
}

// EventHub ...
type EventHub struct {
	client   *Client
	eventHub *gocentrifuge.EventHub
}

// NewEventHub ...
func NewEventHub() *EventHub {
	return &EventHub{}
}

func (h *EventHub) setClient(c *Client) {
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

func (p *eventProxy) OnPrivateSub(c *gocentrifuge.Client, e gocentrifuge.PrivateSubEvent) (string, error) {
	token, err := p.onPrivateSub.OnPrivateSub(p.client, &PrivateSubEvent{
		ClientID: e.ClientID,
		Channel:  e.Channel,
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

func (p *eventProxy) OnRefresh(c *gocentrifuge.Client) (string, error) {
	token, err := p.onRefresh.OnRefresh(p.client)
	if err != nil {
		return "", err
	}
	return token, nil
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
func (c *Client) OnConnect(handler ConnectHandler) {
	proxy := &eventProxy{client: c, onConnect: handler}
	c.client.OnConnect(proxy)
}

// OnDisconnect is a function to handle disconnect event.
func (c *Client) OnDisconnect(handler DisconnectHandler) {
	proxy := &eventProxy{client: c, onDisconnect: handler}
	c.client.OnDisconnect(proxy)
}

// OnPrivateSub needed to handle private channel subscriptions.
func (c *Client) OnPrivateSub(handler PrivateSubHandler) {
	proxy := &eventProxy{client: c, onPrivateSub: handler}
	c.client.OnPrivateSub(proxy)
}

// OnRefresh handles refresh event when client's credentials expired and must be refreshed.
func (c *Client) OnRefresh(handler RefreshHandler) {
	proxy := &eventProxy{client: c, onRefresh: handler}
	c.client.OnRefresh(proxy)
}

// OnError is a function that will receive unhandled errors for logging.
func (c *Client) OnError(handler ErrorHandler) {
	proxy := &eventProxy{client: c, onError: handler}
	c.client.OnError(proxy)
}

// OnMessage allows to process async message from server to client.
func (c *Client) OnMessage(handler MessageHandler) {
	proxy := &eventProxy{client: c, onMessage: handler}
	c.client.OnMessage(proxy)
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
	OnPublish(*Subscription, *PublishEvent)
}

// JoinHandler is a function to handle join messages.
type JoinHandler interface {
	OnJoin(*Subscription, *JoinEvent)
}

// LeaveHandler is a function to handle leave messages.
type LeaveHandler interface {
	OnLeave(*Subscription, *LeaveEvent)
}

// UnsubscribeHandler is a function to handle unsubscribe event.
type UnsubscribeHandler interface {
	OnUnsubscribe(*Subscription, *UnsubscribeEvent)
}

// SubscribeSuccessHandler is a function to handle subscribe success
// event.
type SubscribeSuccessHandler interface {
	OnSubscribeSuccess(*Subscription, *SubscribeSuccessEvent)
}

// SubscribeErrorHandler is a function to handle subscribe error event.
type SubscribeErrorHandler interface {
	OnSubscribeError(*Subscription, *SubscribeErrorEvent)
}

// SubscriptionEventHub ...
type SubscriptionEventHub struct {
	sub         *Subscription
	subEventHub *gocentrifuge.SubscriptionEventHub
}

// NewSubscriptionEventHub initializes new SubscriptionEventHub.
func NewSubscriptionEventHub() *SubscriptionEventHub {
	return &SubscriptionEventHub{}
}

func (h *SubscriptionEventHub) setSub(s *Subscription) {
	h.sub = s
}

type subEventProxy struct {
	sub *Subscription

	onPublish          PublishHandler
	onJoin             JoinHandler
	onLeave            LeaveHandler
	onUnsubscribe      UnsubscribeHandler
	onSubscribeSuccess SubscribeSuccessHandler
	onSubscribeError   SubscribeErrorHandler
}

func (p *subEventProxy) OnPublish(s *gocentrifuge.Subscription, e gocentrifuge.PublishEvent) {
	fmt.Printf("%#v\n", p)
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

func (p *subEventProxy) OnJoin(s *gocentrifuge.Subscription, e gocentrifuge.JoinEvent) {
	p.onJoin.OnJoin(p.sub, &JoinEvent{
		Client:   e.Client,
		User:     e.User,
		ConnInfo: e.ConnInfo,
		ChanInfo: e.ChanInfo,
	})
}

func (p *subEventProxy) OnLeave(s *gocentrifuge.Subscription, e gocentrifuge.LeaveEvent) {
	p.onLeave.OnLeave(p.sub, &LeaveEvent{
		Client:   e.Client,
		User:     e.User,
		ConnInfo: e.ConnInfo,
		ChanInfo: e.ChanInfo,
	})
}

func (p *subEventProxy) OnUnsubscribe(s *gocentrifuge.Subscription, e gocentrifuge.UnsubscribeEvent) {
	p.onUnsubscribe.OnUnsubscribe(p.sub, &UnsubscribeEvent{})
}

func (p *subEventProxy) OnSubscribeSuccess(s *gocentrifuge.Subscription, e gocentrifuge.SubscribeSuccessEvent) {
	p.onSubscribeSuccess.OnSubscribeSuccess(p.sub, &SubscribeSuccessEvent{
		Resubscribed: e.Resubscribed,
		Recovered:    e.Recovered,
	})
}

func (p *subEventProxy) OnSubscribeError(s *gocentrifuge.Subscription, e gocentrifuge.SubscribeErrorEvent) {
	p.onSubscribeError.OnSubscribeError(p.sub, &SubscribeErrorEvent{
		Error: e.Error,
	})
}

// OnPublish allows to set PublishHandler to SubEventHandler.
func (s *Subscription) OnPublish(handler PublishHandler) {
	proxy := &subEventProxy{sub: s, onPublish: handler}
	s.sub.OnPublish(proxy)
}

// OnJoin allows to set JoinHandler to SubEventHandler.
func (s *Subscription) OnJoin(handler JoinHandler) {
	proxy := &subEventProxy{sub: s, onJoin: handler}
	s.sub.OnJoin(proxy)
}

// OnLeave allows to set LeaveHandler to SubEventHandler.
func (s *Subscription) OnLeave(handler LeaveHandler) {
	proxy := &subEventProxy{sub: s, onLeave: handler}
	s.sub.OnLeave(proxy)
}

// OnUnsubscribe allows to set UnsubscribeHandler to SubEventHandler.
func (s *Subscription) OnUnsubscribe(handler UnsubscribeHandler) {
	proxy := &subEventProxy{sub: s, onUnsubscribe: handler}
	s.sub.OnUnsubscribe(proxy)
}

// OnSubscribeSuccess allows to set SubscribeSuccessHandler to SubEventHandler.
func (s *Subscription) OnSubscribeSuccess(handler SubscribeSuccessHandler) {
	proxy := &subEventProxy{sub: s, onSubscribeSuccess: handler}
	s.sub.OnSubscribeSuccess(proxy)
}

// OnSubscribeError allows to set SubscribeErrorHandler to SubEventHandler.
func (s *Subscription) OnSubscribeError(handler SubscribeErrorHandler) {
	proxy := &subEventProxy{sub: s, onSubscribeError: handler}
	s.sub.OnSubscribeError(proxy)
}
