package centrifuge

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jpillora/backoff"
)

// Timestamp is helper function to get current timestamp as
// string - i.e. in a format Centrifugo expects. Actually in
// most cases you need an analogue of this function on your
// app backend when generating client connection token.
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

// NewCredentials initializes Credentials.
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
	ErrDuplicateWaiter      = errors.New("waiter with specified uid already exists")
	ErrWaiterClosed         = errors.New("waiter closed")
	ErrClientStatus         = errors.New("wrong client status to make operation")
	ErrClientDisconnected   = errors.New("client disconnected")
	ErrClientExpired        = errors.New("client expired")
	ErrReconnectFailed      = errors.New("reconnect failed")
	ErrBadSubscribeStatus   = errors.New("bad subscribe status")
	ErrBadUnsubscribeStatus = errors.New("bad unsubscribe status")
	ErrBadPublishStatus     = errors.New("bad publish status")
)

const (
	DefaultPrivateChannelPrefix = "$"
	DefaultTimeoutMilliseconds  = 5000
	DefaultPingMilliseconds     = 25000
	DefaultPongMilliseconds     = 10000
)

// Config contains various client options.
type Config struct {
	TimeoutMilliseconds  int
	PrivateChannelPrefix string
	WebsocketCompression bool
	Ping                 bool
	PingMilliseconds     int
	PongMilliseconds     int
}

// DefaultConfig returns Config with default options.
func DefaultConfig() *Config {
	return &Config{
		Ping:                 true,
		PingMilliseconds:     DefaultPingMilliseconds,
		PongMilliseconds:     DefaultPongMilliseconds,
		PrivateChannelPrefix: DefaultPrivateChannelPrefix,
		TimeoutMilliseconds:  DefaultTimeoutMilliseconds,
		WebsocketCompression: false,
	}
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

// ConnectContext is a connect event context passed to OnConnect callback.
type ConnectContext struct {
	ClientID string
}

// DisconnectContext is a disconnect event context passed to OnDisconnect callback.
type DisconnectContext struct {
	Reason    string
	Reconnect bool
}

// ErrorContext is an error event context passed to OnError callback.
type ErrorContext struct {
	Error string
}

// ConnectHandler is an interface describing how to handle connect event.
type ConnectHandler interface {
	OnConnect(*Client, *ConnectContext)
}

// DisconnectHandler is an interface describing how to handle disconnect event.
type DisconnectHandler interface {
	OnDisconnect(*Client, *DisconnectContext)
}

// PrivateSubHandler is an interface describing how to handle private subscription request.
type PrivateSubHandler interface {
	OnPrivateSub(*Client, *PrivateRequest) (*PrivateSign, error)
}

// RefreshHandler is an interface describing how to connection credentials refresh event.
type RefreshHandler interface {
	OnRefresh(*Client) (*Credentials, error)
}

// ErrorHandler is an interface describing how to handle error event.
type ErrorHandler interface {
	OnError(*Client, *ErrorContext)
}

// EventHandler has all event handlers for client.
type EventHandler struct {
	onConnect    ConnectHandler
	onDisconnect DisconnectHandler
	onPrivateSub PrivateSubHandler
	onRefresh    RefreshHandler
	onError      ErrorHandler
}

// NewEventHandler initializes new EventHandler.
func NewEventHandler() *EventHandler {
	return &EventHandler{}
}

// OnConnect is a function to handle connect event.
func (h *EventHandler) OnConnect(handler ConnectHandler) {
	h.onConnect = handler
}

// OnDisconnect is a function to handle disconnect event.
func (h *EventHandler) OnDisconnect(handler DisconnectHandler) {
	h.onDisconnect = handler
}

// OnPrivateSub needed to handle private channel subscriptions.
func (h *EventHandler) OnPrivateSub(handler PrivateSubHandler) {
	h.onPrivateSub = handler
}

// OnRefresh handles refresh event when client's credentials expired and must be refreshed.
func (h *EventHandler) OnRefresh(handler RefreshHandler) {
	h.onRefresh = handler
}

// OnError is a function to handle critical protocol errors manually.
func (h *EventHandler) OnError(handler ErrorHandler) {
	h.onError = handler
}

const (
	DISCONNECTED = iota
	CONNECTING
	CONNECTED
	CLOSED
)

// Client describes client connection to Centrifugo server.
type Client struct {
	mutex             sync.RWMutex
	url               string
	config            *Config
	credentials       *Credentials
	conn              connection
	msgID             int32
	status            int
	id                string
	subsMutex         sync.RWMutex
	subs              map[string]*Sub
	waitersMutex      sync.RWMutex
	waiters           map[string]chan response
	receive           chan []byte
	write             chan []byte
	closed            chan struct{}
	reconnect         bool
	reconnectStrategy reconnectStrategy
	createConn        connFactory
	events            *EventHandler
	delayPing         chan struct{}
}

func (c *Client) nextMsgID() int32 {
	return atomic.AddInt32(&c.msgID, 1)
}

// New initializes Client struct. It accepts URL to Centrifugo server,
// connection Credentials, EventHandler and Config.
func New(u string, creds *Credentials, events *EventHandler, config *Config) *Client {
	c := &Client{
		url:               u,
		subs:              make(map[string]*Sub),
		config:            config,
		credentials:       creds,
		waiters:           make(map[string]chan response),
		reconnect:         true,
		reconnectStrategy: defaultBackoffReconnect,
		createConn:        newWSConnection,
		events:            events,
		delayPing:         make(chan struct{}, 32),
	}
	return c
}

func (c *Client) subscribed(channel string) bool {
	c.subsMutex.RLock()
	_, ok := c.subs[channel]
	c.subsMutex.RUnlock()
	return ok
}

// clientID returns client ID of this connection. It only available after
// connection was established and authorized.
func (c *Client) clientID() string {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.id
}

func (c *Client) handleError(err error) {
	var handler ErrorHandler
	if c.events != nil && c.events.onError != nil {
		handler = c.events.onError
	}
	if handler != nil {
		handler.OnError(c, &ErrorContext{Error: err.Error()})
	}
}

// Close closes Client connection and cleans ups everything.
func (c *Client) Close() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.status == CONNECTED {
		for ch, sub := range c.subs {
			c.unsubscribe(sub.Channel())
			delete(c.subs, ch)
		}
	}
	c.close()
	c.status = CLOSED
}

// close clean ups ws connection and all outgoing requests.
// Instance Lock must be held outside.
func (c *Client) close() {
	c.waitersMutex.Lock()
	for uid, ch := range c.waiters {
		close(ch)
		delete(c.waiters, uid)
	}
	c.waitersMutex.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

func extractAdvice(err error) *disconnectAdvice {
	adv := &disconnectAdvice{
		Reason:    "connection closed",
		Reconnect: true,
	}
	if err != nil {
		ok, _, reason := closeErr(err)
		if ok {
			var closeAdvice disconnectAdvice
			err := json.Unmarshal([]byte(reason), &closeAdvice)
			if err == nil {
				adv.Reason = closeAdvice.Reason
				adv.Reconnect = closeAdvice.Reconnect
			}
		}
	}
	return adv
}

func (c *Client) handleDisconnect(adv *disconnectAdvice) {
	c.mutex.Lock()
	c.reconnect = adv.Reconnect

	if c.status == DISCONNECTED || c.status == CLOSED {
		if c.conn != nil {
			c.conn.Close()
			c.conn = nil
		}
		c.mutex.Unlock()
		return
	}

	c.waitersMutex.Lock()
	for uid, ch := range c.waiters {
		close(ch)
		delete(c.waiters, uid)
	}
	c.waitersMutex.Unlock()

	close(c.closed)
	c.status = DISCONNECTED

	for _, s := range c.subs {
		s.triggerOnUnsubscribe(true)
	}

	reconnect := c.reconnect
	c.mutex.Unlock()

	var handler DisconnectHandler
	if c.events != nil && c.events.onDisconnect != nil {
		handler = c.events.onDisconnect
	}

	if handler != nil {
		ctx := &DisconnectContext{Reason: adv.Reason, Reconnect: reconnect}
		handler.OnDisconnect(c, ctx)
	}

	if !reconnect {
		return
	}
	err := c.reconnectStrategy.reconnect(c)
	if err != nil {
		c.Close()
	}
}

type reconnectStrategy interface {
	reconnect(c *Client) error
}

type backoffReconnect struct {
	// NumReconnect is maximum number of reconnect attempts, 0 means reconnect forever.
	NumReconnect int
	// Factor is the multiplying factor for each increment step.
	Factor float64
	// Jitter eases contention by randomizing backoff steps.
	Jitter bool
	// MinMilliseconds is a minimum value of the reconnect interval.
	MinMilliseconds int
	// MaxMilliseconds is a maximum value of the reconnect interval.
	MaxMilliseconds int
}

var defaultBackoffReconnect = &backoffReconnect{
	NumReconnect:    0,
	MinMilliseconds: 100,
	MaxMilliseconds: 10 * 1000,
	Factor:          2,
	Jitter:          true,
}

func (r *backoffReconnect) reconnect(c *Client) error {
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

		reconnects++
		err := c.doReconnect()
		if err != nil {
			continue
		}

		// successfully reconnected
		return nil
	}
	return ErrReconnectFailed
}

func (c *Client) doReconnect() error {

	err := c.connect()
	if err != nil {
		c.close()
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

func (c *Client) pinger(closeCh chan struct{}) {
	timeout := time.Duration(c.config.PingMilliseconds) * time.Millisecond
	for {
		select {
		case <-c.delayPing:
		case <-time.After(timeout):
			err := c.sendPing()
			if err != nil {
				adv := extractAdvice(err)
				c.handleDisconnect(adv)
				return
			}
		case <-closeCh:
			return
		}
	}
}

func (c *Client) reader(conn connection, closeCh chan struct{}) {
	for {
		message, err := conn.ReadMessage()
		if err != nil {
			adv := extractAdvice(err)
			c.handleDisconnect(adv)
			return
		}
		select {
		case <-closeCh:
			return
		default:
			select {
			case c.delayPing <- struct{}{}:
			default:
			}
			err := c.handle(message)
			if err != nil {
				c.handleError(err)
			}
		}
	}
}

func (c *Client) writer(conn connection, closeCh chan struct{}) {
	for {
		select {
		case msg := <-c.write:
			err := conn.WriteMessage(msg)
			if err != nil {
				adv := extractAdvice(err)
				c.handleDisconnect(adv)
				return
			}
		case <-closeCh:
			return
		}
	}
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
	body := resp.Body
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
			return nil
		}
		sub.handleMessage(messageFromRaw(m))
	case "join":
		var b joinLeaveMessage
		err := json.Unmarshal(body, &b)
		if err != nil {
			return nil
		}
		channel := b.Channel
		c.subsMutex.RLock()
		sub, ok := c.subs[string(channel)]
		c.subsMutex.RUnlock()
		if !ok {
			return nil
		}
		sub.handleJoinMessage(clientInfoFromRaw(&b.Data))
	case "leave":
		var b joinLeaveMessage
		err := json.Unmarshal(body, &b)
		if err != nil {
			return nil
		}
		channel := b.Channel
		c.subsMutex.RLock()
		sub, ok := c.subs[string(channel)]
		c.subsMutex.RUnlock()
		if !ok {
			return nil
		}
		sub.handleLeaveMessage(clientInfoFromRaw(&b.Data))
	default:
		return nil
	}
	return nil
}

// Connect connects to Centrifugo and sends connect message to authenticate.
func (c *Client) Connect() error {
	c.mutex.Lock()
	if c.status == CONNECTED || c.status == CONNECTING {
		c.mutex.Unlock()
		return nil
	}
	if c.status == CLOSED {
		c.mutex.Unlock()
		return ErrClientStatus
	}
	c.status = CONNECTING
	c.reconnect = true
	c.mutex.Unlock()

	err := c.connect()
	if err != nil {
		adv := extractAdvice(err)
		c.handleDisconnect(adv)
		return nil
	}
	err = c.resubscribe()
	if err != nil {
		// we need just to close the connection and outgoing requests here
		// but preserve all subscriptions.
		c.close()
		adv := extractAdvice(err)
		c.handleDisconnect(adv)
		return nil
	}

	return nil
}

func (c *Client) connect() error {
	c.mutex.Lock()
	if c.status == CONNECTED {
		c.mutex.Unlock()
		return nil
	}
	c.status = CONNECTING
	c.closed = make(chan struct{})
	c.mutex.Unlock()

	conn, err := c.createConn(c.url, time.Duration(c.config.TimeoutMilliseconds)*time.Millisecond, c.config.WebsocketCompression)
	if err != nil {
		return err
	}

	c.mutex.Lock()
	if c.status == DISCONNECTED {
		c.mutex.Unlock()
		return nil
	}

	c.conn = conn
	closeCh := make(chan struct{})
	c.closed = closeCh
	c.write = make(chan []byte, 64)
	c.receive = make(chan []byte, 64)

	c.mutex.Unlock()

	go c.reader(conn, closeCh)
	go c.writer(conn, closeCh)

	var body connectResponseBody

	body, err = c.sendConnect()
	if err != nil {
		return err
	}

	if body.Expires && body.Expired {
		// Try to refresh credentials and repeat connection attempt.
		err = c.refreshCredentials()
		if err != nil {
			c.Close()
			return err
		}
		body, err = c.sendConnect()
		if err != nil {
			c.Close()
			return err
		}
		if body.Expires && body.Expired {
			c.Close()
			return ErrClientExpired
		}
	}

	c.mutex.Lock()
	c.id = body.Client
	prevStatus := c.status
	c.status = CONNECTED
	c.mutex.Unlock()

	if body.Expires {
		go func(interval int64) {
			tick := time.After(time.Duration(interval) * time.Second)
			select {
			case <-c.closed:
				return
			case <-tick:
				c.sendRefresh()
			}
		}(body.TTL)
	}

	if c.config.Ping {
		go c.pinger(closeCh)
	}

	if c.events != nil && c.events.onConnect != nil && prevStatus != CONNECTED {
		handler := c.events.onConnect
		ctx := &ConnectContext{ClientID: c.clientID()}
		handler.OnConnect(c, ctx)
	}

	return nil
}

func (c *Client) resubscribe() error {
	c.subsMutex.RLock()
	defer c.subsMutex.RUnlock()
	for _, sub := range c.subs {
		err := sub.resubscribe()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) disconnect(reconnect bool) error {
	c.mutex.Lock()
	c.reconnect = reconnect
	c.mutex.Unlock()
	c.handleDisconnect(&disconnectAdvice{
		Reconnect: reconnect,
		Reason:    "clean disconnect",
	})
	return nil
}

// Disconnect client from Centrifugo.
func (c *Client) Disconnect() error {
	c.disconnect(false)
	return nil
}

func (c *Client) timeout() time.Duration {
	return time.Duration(c.config.TimeoutMilliseconds) * time.Millisecond
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
	r, err := c.sendSync(cmd.UID, cmdBytes, c.timeout())
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
				c.sendRefresh()
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
	r, err := c.sendSync(cmd.UID, cmdBytes, c.timeout())
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
			privateReq := newPrivateRequest(c.clientID(), channel)
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

// SubscribeAsync allows to subscribe on channel.
func (c *Client) SubscribeAsync(channel string, events *SubEventHandler) *Sub {
	c.subsMutex.Lock()
	var sub *Sub
	if _, ok := c.subs[channel]; ok {
		sub = c.subs[channel]
		sub.events = events
	} else {
		sub = c.newSub(channel, events)
	}
	c.subs[channel] = sub
	c.subsMutex.Unlock()

	go func() {
		err := sub.resubscribe()
		if err != nil {
			c.disconnect(true)
		}
	}()
	return sub
}

// Subscribe allows to subscribe on channel.
func (c *Client) Subscribe(channel string, events *SubEventHandler) (*Sub, error) {
	c.subsMutex.Lock()
	var sub *Sub
	if _, ok := c.subs[channel]; ok {
		sub = c.subs[channel]
		sub.events = events
	} else {
		sub = c.newSub(channel, events)
	}
	c.subs[channel] = sub
	c.subsMutex.Unlock()

	err := sub.resubscribe()
	return sub, err
}

func (c *Client) subscribe(sub *Sub) error {

	channel := sub.Channel()

	privateSign, err := c.privateSign(channel)
	if err != nil {
		return err
	}
	sub.lastMessageMu.Lock()
	body, err := c.sendSubscribe(channel, sub.lastMessageID, privateSign)
	sub.lastMessageMu.Unlock()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if err != nil {
		c.subsMutex.Lock()
		delete(c.subs, channel)
		c.subsMutex.Unlock()
		return err
	}
	if !body.Status {
		c.subsMutex.Lock()
		delete(c.subs, channel)
		c.subsMutex.Unlock()
		return ErrBadSubscribeStatus
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
	return nil
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
		params.Client = c.clientID()
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
	r, err := c.sendSync(cmd.UID, cmdBytes, c.timeout())
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
	raw := json.RawMessage(data)
	params := publishParams{
		Channel: channel,
		Data:    &raw,
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
	r, err := c.sendSync(cmd.UID, cmdBytes, c.timeout())
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
	r, err := c.sendSync(cmd.UID, cmdBytes, c.timeout())
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
	r, err := c.sendSync(cmd.UID, cmdBytes, c.timeout())
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
	r, err := c.sendSync(cmd.UID, cmdBytes, c.timeout())
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

func (c *Client) sendPing() error {
	cmd := pingClientCommand{
		clientCommand: clientCommand{
			UID:    strconv.Itoa(int(c.nextMsgID())),
			Method: "ping",
		},
	}
	cmdBytes, err := json.Marshal(cmd)
	if err != nil {
		return err
	}
	r, err := c.sendSync(cmd.UID, cmdBytes, time.Duration(c.config.PongMilliseconds)*time.Millisecond)
	if err != nil {
		return err
	}
	if r.Error != "" {
		return errors.New(r.Error)
	}
	return nil
}

func (c *Client) sendSync(uid string, msg []byte, timeout time.Duration) (response, error) {
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
	return c.wait(wait, timeout)
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

func (c *Client) wait(ch chan response, timeout time.Duration) (response, error) {
	select {
	case data, ok := <-ch:
		if !ok {
			return response{}, ErrWaiterClosed
		}
		return data, nil
	case <-time.After(timeout):
		return response{}, ErrTimeout
	case <-c.closed:
		return response{}, ErrClientDisconnected
	}
}
