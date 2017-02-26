package centrifuge

import (
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jpillora/backoff"
)

// Timestamp is helper function to get current timestamp as string.
func Timestamp() string {
	return strconv.FormatInt(time.Now().Unix(), 10)
}

// Credentials describe client connection parameters.
type Credentials struct {
	User      string
	Timestamp string
	Info      string
	Token     string
}

func NewCredentials(user, timestamp, info, token string) *Credentials {
	return &Credentials{
		User:      user,
		Timestamp: timestamp,
		Info:      info,
		Token:     token,
	}
}

var (
	ErrTimeout              = errors.New("timed out")
	ErrInvalidMessage       = errors.New("invalid message")
	ErrDuplicateWaiter      = errors.New("waiter with uid already exists")
	ErrWaiterClosed         = errors.New("waiter closed")
	ErrClientStatus         = errors.New("wrong client status to make operation")
	ErrClientDisconnected   = errors.New("client disconnected")
	ErrClientExpired        = errors.New("client expired")
	ErrReconnectForbidden   = errors.New("reconnect is not allowed after disconnect message")
	ErrReconnectFailed      = errors.New("reconnect failed")
	ErrBadSubscribeStatus   = errors.New("bad subscribe status")
	ErrBadUnsubscribeStatus = errors.New("bad unsubscribe status")
	ErrBadPublishStatus     = errors.New("bad publish status")
)

const (
	DefaultPrivateChannelPrefix = "$"
	DefaultTimeoutMilliseconds  = 1 * 10e3
	DefaultReconnect            = true
)

// Config contains various client options.
type Config struct {
	TimeoutMilliseconds  int
	PrivateChannelPrefix string
	Debug                bool
	Reconnect            bool
}

// DefaultConfig with standard private channel prefix and 1 second timeout.
var defaultConfig = &Config{
	PrivateChannelPrefix: DefaultPrivateChannelPrefix,
	TimeoutMilliseconds:  DefaultTimeoutMilliseconds,
	Reconnect:            DefaultReconnect,
}

func DefaultConfig() *Config {
	return defaultConfig
}

// Private sign confirmes that client can subscribe on private channel.
type PrivateSign struct {
	Sign string
	Info string
}

// PrivateRequest contains info required to create PrivateSign when client
// wants to subscribe on private channel.
type PrivateRequest struct {
	ClientID string
	Channel  string
}

func newPrivateRequest(client string, channel string) *PrivateRequest {
	return &PrivateRequest{
		ClientID: client,
		Channel:  channel,
	}
}

type PrivateSubHandler interface {
	OnPrivateSub(*Client, *PrivateRequest) (*PrivateSign, error)
}

type RefreshHandler interface {
	OnRefresh(*Client) (*Credentials, error)
}

type DisconnectHandler interface {
	OnDisconnect(*Client) error
}

type ErrorHandler interface {
	OnError(*Client, error)
}

type EventHandler struct {
	onPrivateSub PrivateSubHandler
	onError      ErrorHandler
	onDisconnect DisconnectHandler
	onRefresh    RefreshHandler
}

func NewEventHandler() *EventHandler {
	return &EventHandler{}
}

// OnDisconnect is a function to handle disconnect event.
func (h *EventHandler) OnDisconnect(handler DisconnectHandler) {
	h.onDisconnect = handler
}

// OnPrivateSub needed to handle private channel subscriptions.
func (h *EventHandler) OnPrivateSub(handler PrivateSubHandler) {
	h.onPrivateSub = handler
}

// OnError is a function to handle critical protocol errors manually.
func (h *EventHandler) OnError(handler ErrorHandler) {
	h.onError = handler
}

// OnRefresh handles refresh event when client's credentials expired and must be refreshed.
func (h *EventHandler) OnRefresh(handler RefreshHandler) {
	h.onRefresh = handler
}

const (
	DISCONNECTED = iota
	CONNECTED
	CLOSED
	RECONNECTING
)

// Client describes client connection to Centrifugo server.
type Client struct {
	mutex        sync.RWMutex
	url          string
	config       *Config
	credentials  *Credentials
	conn         connection
	msgID        int32
	status       int
	clientID     string
	subsMutex    sync.RWMutex
	subs         map[string]*Sub
	waitersMutex sync.RWMutex
	waiters      map[string]chan response
	receive      chan []byte
	write        chan []byte
	closed       chan struct{}
	reconnect    bool
	createConn   connFactory
	events       *EventHandler
}

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

// SubEventHandler contains callback functions that will be called when
// corresponding event happens with subscription to channel.
type SubEventHandler struct {
	onMessage     MessageHandler
	onJoin        JoinHandler
	onLeave       LeaveHandler
	onUnsubscribe UnsubscribeHandler
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

// History allows to extract channel history.
func (s *Sub) History() ([]Message, error) {
	return s.centrifuge.history(s.channel)
}

// Presence allows to extract presence information for channel.
func (s *Sub) Presence() (map[string]ClientInfo, error) {
	return s.centrifuge.presence(s.channel)
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

func (c *Client) nextMsgID() int32 {
	return atomic.AddInt32(&c.msgID, 1)
}

// NewClient initializes Client struct. It accepts URL to Centrifugo server,
// connection Credentials, event handler and Config.
func New(u string, creds *Credentials, events *EventHandler, config *Config) *Client {
	c := &Client{
		url:         u,
		subs:        make(map[string]*Sub),
		config:      config,
		credentials: creds,
		receive:     make(chan []byte, 64),
		write:       make(chan []byte, 64),
		closed:      make(chan struct{}),
		waiters:     make(map[string]chan response),
		reconnect:   true,
		createConn:  newWSConnection,
		events:      events,
	}
	return c
}

// Connected returns true if client is connected at moment.
func (c *Client) Connected() bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.status == CONNECTED
}

// Subscribed returns true if client subscribed on channel.
func (c *Client) Subscribed(channel string) bool {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.subscribed(channel)
}

func (c *Client) subscribed(channel string) bool {
	c.subsMutex.RLock()
	_, ok := c.subs[channel]
	c.subsMutex.RUnlock()
	return ok
}

// ClientID returns client ID of this connection. It only available after connection
// was established and authorized.
func (c *Client) ClientID() string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return string(c.clientID)
}

func (c *Client) handleError(err error) {
	var handler ErrorHandler
	if c.events != nil && c.events.onError != nil {
		handler = c.events.onError
	}
	if handler != nil {
		handler.OnError(c, err)
	} else {
		log.Println(err)
		c.Close()
	}
}

// Close closes Client connection and clean ups everything.
func (c *Client) Close() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.unsubscribeAll()
	c.close()
	c.status = CLOSED
}

// unsubscribeAll destroys all subscriptions.
// Instance Lock must be held outside.
func (c *Client) unsubscribeAll() {
	if c.conn != nil && c.status == CONNECTED {
		for ch, sub := range c.subs {
			err := c.unsubscribe(sub.Channel())
			if err != nil {
				log.Println(err)
			}
			delete(c.subs, ch)
		}
	}
}

// close clean ups ws connection and all outgoing requests.
// Instance Lock must be held outside.
func (c *Client) close() {
	if c.conn != nil {
		c.conn.Close()
	}

	c.waitersMutex.Lock()
	for uid, ch := range c.waiters {
		close(ch)
		delete(c.waiters, uid)
	}
	c.waitersMutex.Unlock()

	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
}

func (c *Client) handleDisconnect(err error) {
	c.mutex.Lock()

	ok, _, reason := closeErr(err)
	if ok {
		var adv disconnectAdvice
		err := json.Unmarshal([]byte(reason), &adv)
		if err == nil {
			if !adv.Reconnect {
				c.reconnect = false
			}
		}
	}

	if c.status == CLOSED || c.status == RECONNECTING {
		c.mutex.Unlock()
		return
	}

	if c.conn != nil {
		c.conn.Close()
	}

	c.waitersMutex.Lock()
	for uid, ch := range c.waiters {
		close(ch)
		delete(c.waiters, uid)
	}
	c.waitersMutex.Unlock()

	select {
	case <-c.closed:
	default:
		close(c.closed)
	}

	c.status = DISCONNECTED

	var handler DisconnectHandler
	if c.events != nil && c.events.onDisconnect != nil {
		handler = c.events.onDisconnect
	}

	c.mutex.Unlock()

	if handler != nil {
		handler.OnDisconnect(c)
	}

}

type BackoffReconnect struct {
	// NumReconnect is maximum number of reconnect attempts, 0 means reconnect forever.
	NumReconnect int
	//Factor is the multiplying factor for each increment step.
	Factor float64
	//Jitter eases contention by randomizing backoff steps.
	Jitter bool
	//Min is a minimum value of the reconnect interval.
	MinMilliseconds int
	//Max is a maximum value of the reconnect interval.
	MaxMilliseconds int
}

var DefaultBackoffReconnect = &BackoffReconnect{
	NumReconnect:    0,
	MinMilliseconds: 100,
	MaxMilliseconds: 10 * 1000,
	Factor:          2,
	Jitter:          true,
}

func (r *BackoffReconnect) reconnect(c *Client) error {
	b := &backoff.Backoff{
		Min:    time.Duration(r.MinMilliseconds) * time.Millisecond,
		Max:    time.Duration(r.MaxMilliseconds) * time.Millisecond,
		Factor: r.Factor,
		Jitter: r.Jitter,
	}
	reconnects := 0
	for {
		if r.NumReconnect > 0 && reconnects >= r.NumReconnect {
			break
		}
		time.Sleep(b.Duration())

		reconnects += 1

		err := c.doReconnect()
		if err != nil {
			log.Println(err)
			continue
		}

		// successfully reconnected
		return nil
	}
	return ErrReconnectFailed
}

func (c *Client) doReconnect() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.closed = make(chan struct{})

	err := c.connect()
	if err != nil {
		close(c.closed)
		return err
	}

	err = c.resubscribe()
	if err != nil {
		// we need just to close the connection and outgoing requests here
		// but preserve all subscriptions.
		c.close()
		return err
	}

	return nil
}

func (c *Client) Reconnect(strategy *BackoffReconnect) error {
	c.mutex.Lock()
	reconnect := c.reconnect
	c.mutex.Unlock()
	if !reconnect {
		return ErrReconnectForbidden
	}
	if strategy == nil {
		strategy = DefaultBackoffReconnect
	}
	c.mutex.Lock()
	c.status = RECONNECTING
	c.mutex.Unlock()
	return strategy.reconnect(c)
}

func (c *Client) resubscribe() error {
	for _, sub := range c.subs {
		err := sub.resubscribe()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) read() {
	for {
		message, err := c.conn.ReadMessage()
		if err != nil {
			c.handleDisconnect(err)
			return
		}
		select {
		case <-c.closed:
			return
		default:
			c.receive <- message
		}
	}
}

func (c *Client) run() {
	for {
		select {
		case msg := <-c.receive:
			err := c.handle(msg)
			if err != nil {
				c.handleError(err)
			}
		case msg := <-c.write:
			err := c.conn.WriteMessage(msg)
			if err != nil {
				c.handleError(err)
			}
		case <-c.closed:
			return
		}
	}
}

var (
	arrayJsonPrefix  byte = '['
	objectJsonPrefix byte = '{'
)

func responsesFromClientMsg(msg []byte) ([]response, error) {
	var resps []response
	firstByte := msg[0]
	switch firstByte {
	case objectJsonPrefix:
		// single command request
		var resp response
		err := json.Unmarshal(msg, &resp)
		if err != nil {
			return nil, err
		}
		resps = append(resps, resp)
	case arrayJsonPrefix:
		// array of commands received
		err := json.Unmarshal(msg, &resps)
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrInvalidMessage
	}
	return resps, nil
}

func (c *Client) handle(msg []byte) error {
	if len(msg) == 0 {
		return nil
	}
	resps, err := responsesFromClientMsg(msg)
	if err != nil {
		return err
	}
	for _, resp := range resps {
		if resp.UID != "" {
			c.waitersMutex.RLock()
			if waiter, ok := c.waiters[resp.UID]; ok {
				waiter <- resp
			}
			c.waitersMutex.RUnlock()
		} else {
			err := c.handleAsyncResponse(resp)
			if err != nil {
				c.handleError(err)
			}
		}
	}
	return nil
}

func (c *Client) handleAsyncResponse(resp response) error {
	method := resp.Method
	errorStr := resp.Error
	body := resp.Body
	if errorStr != "" {
		// Should never occur in usual workflow.
		return errors.New(errorStr)
	}
	switch method {
	case "message":
		var m *rawMessage
		err := json.Unmarshal(body, &m)
		if err != nil {
			// Malformed message received.
			return errors.New("malformed message received from server")
		}
		channel := m.Channel
		c.subsMutex.RLock()
		sub, ok := c.subs[string(channel)]
		c.subsMutex.RUnlock()
		if !ok {
			log.Println("message received but client not subscribed on channel")
			return nil
		}
		sub.handleMessage(messageFromRaw(m))
	case "join":
		var b joinLeaveMessage
		err := json.Unmarshal(body, &b)
		if err != nil {
			log.Println("malformed join message")
			return nil
		}
		channel := b.Channel
		c.subsMutex.RLock()
		sub, ok := c.subs[string(channel)]
		c.subsMutex.RUnlock()
		if !ok {
			log.Printf("join received but client not subscribed on channel: %s", string(channel))
			return nil
		}
		sub.handleJoinMessage(clientInfoFromRaw(&b.Data))
	case "leave":
		var b joinLeaveMessage
		err := json.Unmarshal(body, &b)
		if err != nil {
			log.Println("malformed leave message")
			return nil
		}
		channel := b.Channel
		c.subsMutex.RLock()
		sub, ok := c.subs[string(channel)]
		c.subsMutex.RUnlock()
		if !ok {
			log.Printf("leave received but client not subscribed on channel: %s", string(channel))
			return nil
		}
		sub.handleLeaveMessage(clientInfoFromRaw(&b.Data))
	default:
		return nil
	}
	return nil
}

// Lock must be held outside
func (c *Client) connectWS() error {
	conn, err := c.createConn(c.url, time.Duration(c.config.TimeoutMilliseconds)*time.Millisecond)
	if err != nil {
		return err
	}
	c.conn = conn
	return nil
}

// Lock must be held outside
func (c *Client) connect() error {
	err := c.connectWS()
	if err != nil {
		return err
	}

	go c.run()

	go c.read()

	var body connectResponseBody

	body, err = c.sendConnect()
	if err != nil {
		return err
	}

	if body.Expires && body.Expired {
		// Try to refresh credentials and repeat connection attempt.
		err = c.refreshCredentials()
		if err != nil {
			return err
		}
		body, err = c.sendConnect()
		if err != nil {
			return err
		}
		if body.Expires && body.Expired {
			return ErrClientExpired
		}
	}

	c.clientID = body.Client

	if body.Expires {
		go func(interval int64) {
			tick := time.After(time.Duration(interval) * time.Second)
			select {
			case <-c.closed:
				return
			case <-tick:
				err := c.sendRefresh()
				if err != nil {
					log.Println(err)
				}
			}
		}(body.TTL)
	}

	c.status = CONNECTED

	return nil
}

// Connect connects to Centrifugo and sends connect message to authorize.
func (c *Client) Connect() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.status == CONNECTED {
		return ErrClientStatus
	}
	return c.connect()
}

func (c *Client) refreshCredentials() error {
	var handler RefreshHandler
	if c.events != nil && c.events.onRefresh != nil {
		handler = c.events.onRefresh
	}
	if handler == nil {
		return errors.New("RefreshHandler must be set to handle expired credentials")
	}

	creds, err := handler.OnRefresh(c)
	if err != nil {
		return err
	}
	c.credentials = creds
	return nil
}

func (c *Client) sendRefresh() error {

	err := c.refreshCredentials()
	if err != nil {
		return err
	}

	cmd := refreshClientCommand{
		clientCommand: clientCommand{
			UID:    strconv.Itoa(int(c.nextMsgID())),
			Method: "refresh",
		},
		Params: refreshParams{
			User:      c.credentials.User,
			Timestamp: c.credentials.Timestamp,
			Info:      c.credentials.Info,
			Token:     c.credentials.Token,
		},
	}
	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	r, err := c.sendSync(cmd.UID, cmdBytes)
	if err != nil {
		return err
	}
	if r.Error != "" {
		return errors.New(r.Error)
	}
	var body connectResponseBody
	err = json.Unmarshal(r.Body, &body)
	if err != nil {
		return err
	}
	if body.Expires {
		if body.Expired {
			return ErrClientExpired
		}
		go func(interval int64) {
			tick := time.After(time.Duration(interval) * time.Second)
			select {
			case <-c.closed:
				return
			case <-tick:
				err := c.sendRefresh()
				if err != nil {
					log.Println(err)
				}
			}
		}(body.TTL)
	}
	return nil
}

func (c *Client) sendConnect() (connectResponseBody, error) {
	cmd := connectClientCommand{
		clientCommand: clientCommand{
			UID:    strconv.Itoa(int(c.nextMsgID())),
			Method: "connect",
		},
		Params: connectParams{
			User:      c.credentials.User,
			Timestamp: c.credentials.Timestamp,
			Info:      c.credentials.Info,
			Token:     c.credentials.Token,
		},
	}
	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return connectResponseBody{}, err
	}
	r, err := c.sendSync(cmd.UID, cmdBytes)
	if err != nil {
		return connectResponseBody{}, err
	}
	if r.Error != "" {
		return connectResponseBody{}, errors.New(r.Error)
	}
	var body connectResponseBody
	err = json.Unmarshal(r.Body, &body)
	if err != nil {
		return connectResponseBody{}, err
	}
	return body, nil
}

func (c *Client) privateSign(channel string) (*PrivateSign, error) {
	var ps *PrivateSign
	var err error
	if strings.HasPrefix(channel, c.config.PrivateChannelPrefix) && c.events != nil {
		handler := c.events.onPrivateSub
		if handler != nil {
			privateReq := newPrivateRequest(c.ClientID(), channel)
			ps, err = handler.OnPrivateSub(c, privateReq)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, errors.New("PrivateSubHandler must be set to handle private channel subscriptions")
		}
	}
	return ps, nil
}

// Subscribe allows to subscribe on channel.
func (c *Client) Subscribe(channel string, events *SubEventHandler) (*Sub, error) {
	if !c.Connected() {
		return nil, ErrClientDisconnected
	}
	privateSign, err := c.privateSign(channel)
	if err != nil {
		return nil, err
	}
	c.subsMutex.Lock()
	sub := c.newSub(channel, events)
	c.subs[channel] = sub
	c.subsMutex.Unlock()

	sub.lastMessageMu.Lock()
	body, err := c.sendSubscribe(channel, sub.lastMessageID, privateSign)
	sub.lastMessageMu.Unlock()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if err != nil {
		c.subsMutex.Lock()
		delete(c.subs, channel)
		c.subsMutex.Unlock()
		return nil, err
	}
	if !body.Status {
		c.subsMutex.Lock()
		delete(c.subs, channel)
		c.subsMutex.Unlock()
		return nil, ErrBadSubscribeStatus
	}

	if len(body.Messages) > 0 {
		for i := len(body.Messages) - 1; i >= 0; i-- {
			sub.handleMessage(messageFromRaw(&body.Messages[i]))
		}
	} else {
		lastID := string(body.Last)
		sub.lastMessageMu.Lock()
		sub.lastMessageID = &lastID
		sub.lastMessageMu.Unlock()
	}

	// Subscription on channel successfull.
	return sub, nil
}

func (c *Client) sendSubscribe(channel string, lastMessageID *string, privateSign *PrivateSign) (subscribeResponseBody, error) {

	params := subscribeParams{
		Channel: channel,
	}
	if lastMessageID != nil {
		params.Recover = true
		params.Last = *lastMessageID
	}
	if privateSign != nil {
		params.Client = c.ClientID()
		params.Info = privateSign.Info
		params.Sign = privateSign.Sign
	}

	cmd := subscribeClientCommand{
		clientCommand: clientCommand{
			UID:    strconv.Itoa(int(c.nextMsgID())),
			Method: "subscribe",
		},
		Params: params,
	}
	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return subscribeResponseBody{}, err
	}
	r, err := c.sendSync(cmd.UID, cmdBytes)
	if err != nil {
		return subscribeResponseBody{}, err
	}
	if r.Error != "" {
		return subscribeResponseBody{}, errors.New(r.Error)
	}
	var body subscribeResponseBody
	err = json.Unmarshal(r.Body, &body)
	if err != nil {
		return subscribeResponseBody{}, err
	}
	return body, nil
}

func (c *Client) publish(channel string, data []byte) error {
	body, err := c.sendPublish(channel, data)
	if err != nil {
		return err
	}
	if !body.Status {
		return ErrBadPublishStatus
	}
	return nil
}

func (c *Client) sendPublish(channel string, data []byte) (publishResponseBody, error) {
	params := publishParams{
		Channel: channel,
		Data:    json.RawMessage(data),
	}
	cmd := publishClientCommand{
		clientCommand: clientCommand{
			UID:    strconv.Itoa(int(c.nextMsgID())),
			Method: "publish",
		},
		Params: params,
	}
	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return publishResponseBody{}, err
	}
	r, err := c.sendSync(cmd.UID, cmdBytes)
	if err != nil {
		return publishResponseBody{}, err
	}
	if r.Error != "" {
		return publishResponseBody{}, errors.New(r.Error)
	}
	var body publishResponseBody
	err = json.Unmarshal(r.Body, &body)
	if err != nil {
		return publishResponseBody{}, err
	}
	return body, nil
}

func (c *Client) history(channel string) ([]Message, error) {
	body, err := c.sendHistory(channel)
	if err != nil {
		return []Message{}, err
	}
	messages := make([]Message, len(body.Data))
	for i, m := range body.Data {
		messages[i] = *messageFromRaw(&m)
	}
	return messages, nil
}

func (c *Client) sendHistory(channel string) (historyResponseBody, error) {
	cmd := historyClientCommand{
		clientCommand: clientCommand{
			UID:    strconv.Itoa(int(c.nextMsgID())),
			Method: "history",
		},
		Params: historyParams{
			Channel: channel,
		},
	}
	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return historyResponseBody{}, err
	}
	r, err := c.sendSync(cmd.UID, cmdBytes)
	if err != nil {
		return historyResponseBody{}, err
	}
	if r.Error != "" {
		return historyResponseBody{}, errors.New(r.Error)
	}
	var body historyResponseBody
	err = json.Unmarshal(r.Body, &body)
	if err != nil {
		return historyResponseBody{}, err
	}
	return body, nil
}

func (c *Client) presence(channel string) (map[string]ClientInfo, error) {
	body, err := c.sendPresence(channel)
	if err != nil {
		return map[string]ClientInfo{}, err
	}
	p := make(map[string]ClientInfo)
	for uid, info := range body.Data {
		p[uid] = *clientInfoFromRaw(&info)
	}
	return p, nil
}

func (c *Client) sendPresence(channel string) (presenceResponseBody, error) {
	cmd := presenceClientCommand{
		clientCommand: clientCommand{
			UID:    strconv.Itoa(int(c.nextMsgID())),
			Method: "presence",
		},
		Params: presenceParams{
			Channel: channel,
		},
	}
	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return presenceResponseBody{}, err
	}
	r, err := c.sendSync(cmd.UID, cmdBytes)
	if err != nil {
		return presenceResponseBody{}, err
	}
	if r.Error != "" {
		return presenceResponseBody{}, errors.New(r.Error)
	}
	var body presenceResponseBody
	err = json.Unmarshal(r.Body, &body)
	if err != nil {
		return presenceResponseBody{}, err
	}
	return body, nil
}

func (c *Client) unsubscribe(channel string) error {
	if !c.subscribed(channel) {
		return nil
	}
	body, err := c.sendUnsubscribe(channel)
	if err != nil {
		return err
	}
	if !body.Status {
		return ErrBadUnsubscribeStatus
	}
	c.subsMutex.Lock()
	delete(c.subs, channel)
	c.subsMutex.Unlock()
	return nil
}

func (c *Client) sendUnsubscribe(channel string) (unsubscribeResponseBody, error) {
	cmd := unsubscribeClientCommand{
		clientCommand: clientCommand{
			UID:    strconv.Itoa(int(c.nextMsgID())),
			Method: "unsubscribe",
		},
		Params: unsubscribeParams{
			Channel: channel,
		},
	}
	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return unsubscribeResponseBody{}, err
	}
	r, err := c.sendSync(cmd.UID, cmdBytes)
	if err != nil {
		return unsubscribeResponseBody{}, err
	}
	if r.Error != "" {
		return unsubscribeResponseBody{}, errors.New(r.Error)
	}
	var body unsubscribeResponseBody
	err = json.Unmarshal(r.Body, &body)
	if err != nil {
		return unsubscribeResponseBody{}, err
	}
	return body, nil
}

func (c *Client) sendSync(uid string, msg []byte) (response, error) {
	wait := make(chan response)
	err := c.addWaiter(uid, wait)
	defer c.removeWaiter(uid)
	if err != nil {
		return response{}, err
	}
	err = c.send(msg)
	if err != nil {
		return response{}, err
	}
	return c.wait(wait)
}

func (c *Client) send(msg []byte) error {
	select {
	case <-c.closed:
		return ErrClientDisconnected
	default:
		c.write <- msg
	}
	return nil
}

func (c *Client) addWaiter(uid string, ch chan response) error {
	c.waitersMutex.Lock()
	defer c.waitersMutex.Unlock()
	if _, ok := c.waiters[uid]; ok {
		return ErrDuplicateWaiter
	}
	c.waiters[uid] = ch
	return nil
}

func (c *Client) removeWaiter(uid string) error {
	c.waitersMutex.Lock()
	defer c.waitersMutex.Unlock()
	delete(c.waiters, uid)
	return nil
}

func (c *Client) wait(ch chan response) (response, error) {
	select {
	case data, ok := <-ch:
		if !ok {
			return response{}, ErrWaiterClosed
		}
		return data, nil
	case <-time.After(time.Duration(c.config.TimeoutMilliseconds) * time.Millisecond):
		return response{}, ErrTimeout
	case <-c.closed:
		return response{}, ErrClientDisconnected
	}
}
