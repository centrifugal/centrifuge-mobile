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
// string - i.e. in a format Centrifugo expects.
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
	DefaultTimeoutMilliseconds  = 1 * 10e3
)

// Config contains various client options.
type Config struct {
	TimeoutMilliseconds  int
	PrivateChannelPrefix string
}

// DefaultConfig with standard private channel prefix and 1 second timeout.
var defaultConfig = &Config{
	PrivateChannelPrefix: DefaultPrivateChannelPrefix,
	TimeoutMilliseconds:  DefaultTimeoutMilliseconds,
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

type ConnectHandler interface {
	OnConnect(*Client)
}

type DisconnectHandler interface {
	OnDisconnect(*Client)
}

type PrivateSubHandler interface {
	OnPrivateSub(*Client, *PrivateRequest) (*PrivateSign, error)
}

type RefreshHandler interface {
	OnRefresh(*Client) (*Credentials, error)
}

type ErrorHandler interface {
	OnError(*Client, error)
}

type EventHandler struct {
	onConnect    ConnectHandler
	onDisconnect DisconnectHandler
	onPrivateSub PrivateSubHandler
	onRefresh    RefreshHandler
	onError      ErrorHandler
}

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
	clientID          string
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
		receive:           make(chan []byte, 64),
		write:             make(chan []byte, 64),
		closed:            make(chan struct{}),
		waiters:           make(map[string]chan response),
		reconnect:         true,
		reconnectStrategy: defaultBackoffReconnect,
		createConn:        newWSConnection,
		events:            events,
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
	return c.clientID
}

func (c *Client) handleError(err error) {
	var handler ErrorHandler
	if c.events != nil && c.events.onError != nil {
		handler = c.events.onError
	}
	if handler != nil {
		handler.OnError(c, err)
	} else {
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
			c.unsubscribe(sub.Channel())
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

	if c.status == CLOSED || c.status == CONNECTING {
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

	reconnect := c.reconnect
	c.mutex.Unlock()

	if handler != nil {
		go func() {
			handler.OnDisconnect(c)
		}()
	}

	if !reconnect {
		return
	}
	err = c.reconnectStrategy.reconnect(c)
	if err != nil {
		c.Close()
	} else {
		if c.events != nil && c.events.onConnect != nil {
			handler := c.events.onConnect
			go func() {
				handler.OnConnect(c)
			}()
		}
	}
}

type reconnectStrategy interface {
	reconnect(c *Client) error
}

type backoffReconnect struct {
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

		reconnects += 1
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

// Lock must be held outside
func (c *Client) connect() error {

	c.status = CONNECTING

	conn, err := c.createConn(c.url, time.Duration(c.config.TimeoutMilliseconds)*time.Millisecond)
	if err != nil {
		return err
	}

	c.conn = conn
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
				c.sendRefresh()
			}
		}(body.TTL)
	}

	c.status = CONNECTED

	return nil
}

// Connect connects to Centrifugo and sends connect message to authorize.
func (c *Client) Connect() error {
	c.mutex.Lock()
	if c.status == CONNECTED {
		c.mutex.Unlock()
		return ErrClientStatus
	}
	err := c.connect()
	c.mutex.Unlock()
	if err == nil && c.events != nil && c.events.onConnect != nil {
		handler := c.events.onConnect
		go func() {
			handler.OnConnect(c)
		}()
	}
	return err
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
	c.subsMutex.Lock()
	sub := c.newSub(channel, events)
	c.subs[channel] = sub
	c.subsMutex.Unlock()

	privateSign, err := c.privateSign(channel)
	if err != nil {
		return nil, err
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
