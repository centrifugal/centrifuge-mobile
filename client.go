package centrifuge

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/centrifugal/centrifuge-mobile/proto"
	"github.com/jpillora/backoff"
)

// Exp is helper function to get sign exp timestamp as
// string - i.e. in a format Centrifuge server expects.
// Actually in most cases you need an analogue of this
// function on your app backend when generating client
// connection credentials.
func Exp(ttlSeconds int) string {
	return strconv.FormatInt(time.Now().Unix()+int64(ttlSeconds), 10)
}

// Credentials describe client connection parameters.
type Credentials struct {
	User string
	Exp  string
	Info string
	Sign string
}

// NewCredentials initializes Credentials.
func NewCredentials(user, exp, info, sign string) *Credentials {
	return &Credentials{
		User: user,
		Exp:  exp,
		Info: info,
		Sign: sign,
	}
}

type disconnect struct {
	Reason    string
	Reconnect bool
}

var (
	// ErrTimeout ...
	ErrTimeout            = errors.New("timeout")
	ErrWaiterClosed       = errors.New("waiter closed")
	ErrClientClosed       = errors.New("client closed")
	ErrClientDisconnected = errors.New("client disconnected")
	ErrClientExpired      = errors.New("client connection expired")
	ErrReconnectFailed    = errors.New("reconnect failed")
)

var (
	DefaultPrivateChannelPrefix     = "$"
	DefaultTimeoutMilliseconds      = 5000
	DefaultPingIntervalMilliseconds = 25000
	DefaultPongWaitMilliseconds     = 10000
)

// Config contains various client options.
type Config struct {
	TimeoutMilliseconds      int
	PrivateChannelPrefix     string
	WebsocketCompression     bool
	PingIntervalMilliseconds int
	PongWaitMilliseconds     int
}

// DefaultConfig returns Config with default options.
func DefaultConfig() *Config {
	return &Config{
		PingIntervalMilliseconds: DefaultPingIntervalMilliseconds,
		PongWaitMilliseconds:     DefaultPongWaitMilliseconds,
		PrivateChannelPrefix:     DefaultPrivateChannelPrefix,
		TimeoutMilliseconds:      DefaultTimeoutMilliseconds,
		WebsocketCompression:     false,
	}
}

// PrivateSign confirmes that client can subscribe on private channel.
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
	encoding          proto.Encoding
	config            *Config
	credentials       *Credentials
	conn              connection
	msgID             int32
	status            int
	id                string
	subsMutex         sync.RWMutex
	subs              map[string]*Sub
	waitersMutex      sync.RWMutex
	waiters           map[uint32]chan proto.Reply
	receive           chan []byte
	write             chan []byte
	closed            chan struct{}
	reconnect         bool
	reconnectStrategy reconnectStrategy
	createConn        connFactory
	events            *EventHandler
	delayPing         chan struct{}
	paramsEncoder     proto.ParamsEncoder
	resultDecoder     proto.ResultDecoder
	commandEncoder    proto.CommandEncoder
	messageEncoder    proto.MessageEncoder
	messageDecoder    proto.MessageDecoder
}

func (c *Client) nextMsgID() int32 {
	return atomic.AddInt32(&c.msgID, 1)
}

// New initializes Client struct. It accepts URL to Centrifuge server,
// EventHandler and Config.
func New(u string, events *EventHandler, config *Config) *Client {
	var encoding proto.Encoding
	if strings.Contains(strings.ToLower(u), "format=protobuf") {
		encoding = proto.EncodingProtobuf
	} else {
		encoding = proto.EncodingJSON
	}
	c := &Client{
		url:               u,
		encoding:          encoding,
		subs:              make(map[string]*Sub),
		config:            config,
		waiters:           make(map[uint32]chan proto.Reply),
		reconnect:         true,
		reconnectStrategy: defaultBackoffReconnect,
		createConn:        newWSConnection,
		events:            events,
		delayPing:         make(chan struct{}, 32),
		paramsEncoder:     proto.GetParamsEncoder(encoding),
		resultDecoder:     proto.GetResultDecoder(encoding),
		commandEncoder:    proto.GetCommandEncoder(encoding),
		messageEncoder:    proto.GetMessageEncoder(encoding),
		messageDecoder:    proto.GetMessageDecoder(encoding),
	}
	return c
}

// SetCredentials allows to set credentials to let client
// authenticate itself on connect.
func (c *Client) SetCredentials(creds *Credentials) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.credentials = creds
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
// TODO: check unsubscribe and disconnect called.
func (c *Client) Close() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	// if c.status == CONNECTED {
	// 	c.subsMutex.RLock()
	// 	unsubs := make(map[string]*Sub, len(c.subs))
	// 	for ch, sub := range c.subs {
	// 		unsubs[ch] = sub
	// 	}
	// 	c.subsMutex.RUnlock()
	// 	for ch, sub := range unsubs {
	// 		c.unsubscribe(sub.Channel())
	// 		delete(c.subs, ch)
	// 	}
	// }
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

func extractDisconnect(err error) *disconnect {
	d := &disconnect{
		Reason:    "connection closed",
		Reconnect: true,
	}
	if err != nil {
		ok, _, reason := closeErr(err)
		if ok {
			var disconnectAdvice disconnect
			err := json.Unmarshal([]byte(reason), &disconnectAdvice)
			if err == nil {
				d.Reason = disconnectAdvice.Reason
				d.Reconnect = disconnectAdvice.Reconnect
			}
		}
	}
	return d
}

func (c *Client) handleDisconnect(d *disconnect) {
	c.mutex.Lock()
	c.reconnect = d.Reconnect

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

	select {
	case <-c.closed:
	default:
		close(c.closed)
	}
	c.status = DISCONNECTED

	c.subsMutex.RLock()
	unsubs := make([]*Sub, 0, len(c.subs))
	for _, s := range c.subs {
		unsubs = append(unsubs, s)
	}
	c.subsMutex.RUnlock()

	for _, s := range unsubs {
		s.triggerOnUnsubscribe(true)
	}

	reconnect := c.reconnect
	c.mutex.Unlock()

	var handler DisconnectHandler
	if c.events != nil && c.events.onDisconnect != nil {
		handler = c.events.onDisconnect
	}

	if handler != nil {
		ctx := &DisconnectContext{Reason: d.Reason, Reconnect: reconnect}
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
	timeout := time.Duration(c.config.PingIntervalMilliseconds) * time.Millisecond
	for {
		select {
		case <-c.delayPing:
		case <-time.After(timeout):
			err := c.sendPing()
			if err != nil {
				disconnect := extractDisconnect(err)
				c.handleDisconnect(disconnect)
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
			disconnect := extractDisconnect(err)
			c.handleDisconnect(disconnect)
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
				disconnect := extractDisconnect(err)
				c.handleDisconnect(disconnect)
				return
			}
		case <-closeCh:
			return
		}
	}
}

func (c *Client) handle(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	decoder := proto.GetReplyDecoder(c.encoding, data)
	defer proto.PutReplyDecoder(c.encoding, decoder)

	for {
		reply, err := decoder.Decode()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if reply.ID > 0 {
			c.waitersMutex.RLock()
			if waiter, ok := c.waiters[reply.ID]; ok {
				waiter <- *reply
			}
			c.waitersMutex.RUnlock()
		} else {
			messageDecoder := proto.GetMessageDecoder(c.encoding)
			message, err := messageDecoder.Decode(reply.Result)
			if err != nil {
				c.handleError(err)
				continue
			}
			err = c.handleMessage(*message)
			if err != nil {
				c.handleError(err)
			}
		}

	}

	return nil
}

func (c *Client) handlePush(msg proto.Push) error {
	return nil
}

func (c *Client) handleMessage(msg proto.Message) error {
	messageDecoder := proto.GetMessageDecoder(c.encoding)

	switch msg.Type {
	case proto.MessageTypePush:
		m, err := messageDecoder.DecodePush(msg.Data)
		if err != nil {
			return err
		}
		c.handlePush(*m)
	case proto.MessageTypeUnsub:
		m, err := messageDecoder.DecodeUnsub(msg.Data)
		if err != nil {
			return err
		}
		channel := msg.Channel
		c.subsMutex.RLock()
		sub, ok := c.subs[string(channel)]
		c.subsMutex.RUnlock()
		if !ok {
			return nil
		}
		sub.handleUnsub(*m)
	case proto.MessageTypePub:
		m, err := messageDecoder.DecodePub(msg.Data)
		if err != nil {
			return err
		}
		channel := msg.Channel
		c.subsMutex.RLock()
		sub, ok := c.subs[string(channel)]
		c.subsMutex.RUnlock()
		if !ok {
			return nil
		}
		sub.handlePub(*m)
	case proto.MessageTypeJoin:
		m, err := messageDecoder.DecodeJoin(msg.Data)
		if err != nil {
			return nil
		}
		channel := msg.Channel
		c.subsMutex.RLock()
		sub, ok := c.subs[string(channel)]
		c.subsMutex.RUnlock()
		if !ok {
			return nil
		}
		sub.handleJoin(m.Info)
	case proto.MessageTypeLeave:
		m, err := messageDecoder.DecodeLeave(msg.Data)
		if err != nil {
			return nil
		}
		channel := msg.Channel
		c.subsMutex.RLock()
		sub, ok := c.subs[string(channel)]
		c.subsMutex.RUnlock()
		if !ok {
			return nil
		}
		sub.handleLeave(m.Info)
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
		return ErrClientClosed
	}
	c.status = CONNECTING
	c.reconnect = true
	c.mutex.Unlock()

	err := c.connect()
	if err != nil {
		disconnect := extractDisconnect(err)
		c.handleDisconnect(disconnect)
		return nil
	}
	err = c.resubscribe()
	if err != nil {
		// we need just to close the connection and outgoing requests here
		// but preserve all subscriptions.
		c.close()
		disconnect := extractDisconnect(err)
		c.handleDisconnect(disconnect)
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

	var res proto.ConnectResult

	res, err = c.sendConnect()
	if err != nil {
		return err
	}

	if res.Expires && res.Expired {
		// Try to refresh credentials and repeat connection attempt.
		err = c.refreshCredentials()
		if err != nil {
			c.Close()
			return err
		}
		res, err = c.sendConnect()
		if err != nil {
			c.Close()
			return err
		}
		if res.Expires && res.Expired {
			c.Close()
			return ErrClientExpired
		}
	}

	c.mutex.Lock()
	c.id = res.Client
	prevStatus := c.status
	c.status = CONNECTED
	c.mutex.Unlock()

	if res.Expires {
		go func(interval uint32) {
			tick := time.After(time.Duration(interval) * time.Second)
			select {
			case <-c.closed:
				return
			case <-tick:
				c.sendRefresh()
			}
		}(res.TTL)
	}

	go c.pinger(closeCh)

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
	c.handleDisconnect(&disconnect{
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
	c.mutex.Lock()
	c.credentials = creds
	c.mutex.Unlock()
	return nil
}

func (c *Client) sendRefresh() error {

	err := c.refreshCredentials()
	if err != nil {
		return err
	}

	c.mutex.RLock()
	cmd := &proto.Command{
		ID:     uint32(c.nextMsgID()),
		Method: proto.MethodTypeRefresh,
	}
	params := proto.RefreshRequest{
		User: c.credentials.User,
		Exp:  c.credentials.Exp,
		Info: c.credentials.Info,
		Sign: c.credentials.Sign,
	}
	paramsData, err := c.paramsEncoder.Encode(params)
	if err != nil {
		c.mutex.RUnlock()
		return err
	}
	cmd.Params = paramsData
	c.mutex.RUnlock()

	cmdBytes, err := c.commandEncoder.Encode(cmd)
	if err != nil {
		return err
	}
	r, err := c.sendSync(cmd.ID, cmdBytes, c.timeout())
	if err != nil {
		return err
	}
	if r.Error != nil {
		return r.Error
	}
	var res proto.RefreshResult
	err = c.resultDecoder.Decode(r.Result, &res)
	if err != nil {
		return err
	}
	if res.Expires {
		if res.Expired {
			return ErrClientExpired
		}
		go func(interval uint32) {
			tick := time.After(time.Duration(interval) * time.Second)
			select {
			case <-c.closed:
				return
			case <-tick:
				c.sendRefresh()
			}
		}(res.TTL)
	}
	return nil
}

func (c *Client) sendConnect() (proto.ConnectResult, error) {
	cmd := &proto.Command{
		ID:     uint32(c.nextMsgID()),
		Method: proto.MethodTypeConnect,
	}

	c.mutex.RLock()
	if c.credentials != nil {
		params := proto.ConnectRequest{
			User: c.credentials.User,
			Exp:  c.credentials.Exp,
			Info: c.credentials.Info,
			Sign: c.credentials.Sign,
		}
		paramsData, err := c.paramsEncoder.Encode(params)
		if err != nil {
			return proto.ConnectResult{}, err
		}
		cmd.Params = paramsData
	}
	c.mutex.RUnlock()

	cmdBytes, err := c.commandEncoder.Encode(cmd)
	if err != nil {
		return proto.ConnectResult{}, err
	}
	r, err := c.sendSync(cmd.ID, cmdBytes, c.timeout())
	if err != nil {
		return proto.ConnectResult{}, err
	}
	if r.Error != nil {
		return proto.ConnectResult{}, r.Error
	}

	var res proto.ConnectResult
	err = c.resultDecoder.Decode(r.Result, &res)
	if err != nil {
		return proto.ConnectResult{}, err
	}
	return res, nil
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

// Subscribe allows to subscribe on channel.
func (c *Client) Subscribe(channel string, events *SubEventHandler) *Sub {
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

// SubscribeSync allows to subscribe on channel and wait until subscribe success or error.
func (c *Client) SubscribeSync(channel string, events *SubEventHandler) (*Sub, error) {
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
	res, err := c.sendSubscribe(channel, sub.lastMessageID, privateSign)
	sub.lastMessageMu.Unlock()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if err != nil {
		c.subsMutex.Lock()
		delete(c.subs, channel)
		c.subsMutex.Unlock()
		return err
	}

	if len(res.Pubs) > 0 {
		for i := len(res.Pubs) - 1; i >= 0; i-- {
			sub.handlePub(*res.Pubs[i])
		}
	} else {
		lastID := string(res.Last)
		sub.lastMessageMu.Lock()
		sub.lastMessageID = &lastID
		sub.lastMessageMu.Unlock()
	}
	return nil
}

func (c *Client) sendSubscribe(channel string, lastMessageID *string, privateSign *PrivateSign) (proto.SubscribeResult, error) {
	params := &proto.SubscribeRequest{
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

	paramsData, err := c.paramsEncoder.Encode(params)
	if err != nil {
		return proto.SubscribeResult{}, err
	}

	cmd := &proto.Command{
		ID:     uint32(c.nextMsgID()),
		Method: proto.MethodTypeSubscribe,
		Params: paramsData,
	}
	cmdBytes, err := c.commandEncoder.Encode(cmd)
	if err != nil {
		return proto.SubscribeResult{}, err
	}
	r, err := c.sendSync(cmd.ID, cmdBytes, c.timeout())
	if err != nil {
		return proto.SubscribeResult{}, err
	}
	if r.Error != nil {
		return proto.SubscribeResult{}, r.Error
	}

	var res proto.SubscribeResult
	err = c.resultDecoder.Decode(r.Result, &res)
	if err != nil {
		return proto.SubscribeResult{}, err
	}
	return res, nil
}

func (c *Client) publish(channel string, data []byte) error {
	_, err := c.sendPublish(channel, data)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) sendPublish(channel string, data []byte) (proto.PublishResult, error) {
	params := &proto.PublishRequest{
		Channel: channel,
		Data:    proto.Raw(data),
	}

	paramsData, err := c.paramsEncoder.Encode(params)
	if err != nil {
		return proto.PublishResult{}, err
	}

	cmd := &proto.Command{
		ID:     uint32(c.nextMsgID()),
		Method: proto.MethodTypePublish,
		Params: paramsData,
	}
	cmdBytes, err := c.commandEncoder.Encode(cmd)
	if err != nil {
		return proto.PublishResult{}, err
	}
	r, err := c.sendSync(cmd.ID, cmdBytes, c.timeout())
	if err != nil {
		return proto.PublishResult{}, err
	}
	if r.Error != nil {
		return proto.PublishResult{}, r.Error
	}
	var res proto.PublishResult
	return res, nil
}

func (c *Client) history(channel string) ([]proto.Pub, error) {
	res, err := c.sendHistory(channel)
	if err != nil {
		return []proto.Pub{}, err
	}
	pubs := make([]proto.Pub, len(res.Pubs))
	for i, m := range res.Pubs {
		pubs[i] = *m
	}
	return pubs, nil
}

func (c *Client) sendHistory(channel string) (proto.HistoryResult, error) {
	params := proto.HistoryRequest{
		Channel: channel,
	}

	paramsData, err := c.paramsEncoder.Encode(params)
	if err != nil {
		return proto.HistoryResult{}, err
	}

	cmd := &proto.Command{
		ID:     uint32(c.nextMsgID()),
		Method: proto.MethodTypeHistory,
		Params: paramsData,
	}
	cmdBytes, err := c.commandEncoder.Encode(cmd)
	if err != nil {
		return proto.HistoryResult{}, err
	}
	r, err := c.sendSync(cmd.ID, cmdBytes, c.timeout())
	if err != nil {
		return proto.HistoryResult{}, err
	}
	if r.Error != nil {
		return proto.HistoryResult{}, r.Error
	}
	var res proto.HistoryResult
	err = c.resultDecoder.Decode(r.Result, &res)
	if err != nil {
		return proto.HistoryResult{}, err
	}
	return res, nil
}

func (c *Client) presence(channel string) (map[string]proto.ClientInfo, error) {
	res, err := c.sendPresence(channel)
	if err != nil {
		return map[string]proto.ClientInfo{}, err
	}
	p := make(map[string]proto.ClientInfo)
	for uid, info := range res.Presence {
		p[uid] = *info
	}
	return p, nil
}

func (c *Client) sendPresence(channel string) (proto.PresenceResult, error) {
	params := proto.PresenceRequest{
		Channel: channel,
	}

	paramsData, err := c.paramsEncoder.Encode(params)
	if err != nil {
		return proto.PresenceResult{}, err
	}

	cmd := &proto.Command{
		ID:     uint32(c.nextMsgID()),
		Method: proto.MethodTypePresence,
		Params: paramsData,
	}
	cmdBytes, err := c.commandEncoder.Encode(cmd)
	if err != nil {
		return proto.PresenceResult{}, err
	}
	r, err := c.sendSync(cmd.ID, cmdBytes, c.timeout())
	if err != nil {
		return proto.PresenceResult{}, err
	}
	if r.Error != nil {
		return proto.PresenceResult{}, r.Error
	}
	var res proto.PresenceResult
	err = c.resultDecoder.Decode(r.Result, &res)
	if err != nil {
		return proto.PresenceResult{}, err
	}
	return res, nil
}

func (c *Client) unsubscribe(channel string) error {
	if !c.subscribed(channel) {
		return nil
	}
	_, err := c.sendUnsubscribe(channel)
	if err != nil {
		return err
	}
	return nil
}

func (c *Client) sendUnsubscribe(channel string) (proto.UnsubscribeResult, error) {
	params := proto.PresenceRequest{
		Channel: channel,
	}

	paramsData, err := c.paramsEncoder.Encode(params)
	if err != nil {
		return proto.UnsubscribeResult{}, err
	}

	cmd := &proto.Command{
		ID:     uint32(c.nextMsgID()),
		Method: proto.MethodTypeUnsubscribe,
		Params: paramsData,
	}
	cmdBytes, err := c.commandEncoder.Encode(cmd)
	if err != nil {
		return proto.UnsubscribeResult{}, err
	}
	r, err := c.sendSync(cmd.ID, cmdBytes, c.timeout())
	if err != nil {
		return proto.UnsubscribeResult{}, err
	}
	if r.Error != nil {
		return proto.UnsubscribeResult{}, r.Error
	}
	var res proto.UnsubscribeResult
	err = c.resultDecoder.Decode(r.Result, &res)
	if err != nil {
		return proto.UnsubscribeResult{}, err
	}
	return res, nil
}

func (c *Client) sendPing() error {
	cmd := &proto.Command{
		ID:     uint32(c.nextMsgID()),
		Method: proto.MethodTypePing,
	}
	cmdBytes, err := c.commandEncoder.Encode(cmd)
	if err != nil {
		return err
	}
	r, err := c.sendSync(cmd.ID, cmdBytes, time.Duration(c.config.PongWaitMilliseconds)*time.Millisecond)
	if err != nil {
		return err
	}
	if r.Error != nil {
		return r.Error
	}
	return nil
}

func (c *Client) sendSync(id uint32, msg []byte, timeout time.Duration) (proto.Reply, error) {
	waitCh := make(chan proto.Reply, 1)

	c.addWaiter(id, waitCh)
	defer c.removeWaiter(id)

	err := c.send(msg)
	if err != nil {
		return proto.Reply{}, err
	}
	return c.wait(waitCh, timeout)
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

func (c *Client) addWaiter(id uint32, ch chan proto.Reply) {
	c.waitersMutex.Lock()
	defer c.waitersMutex.Unlock()
	c.waiters[id] = ch
}

func (c *Client) removeWaiter(id uint32) {
	c.waitersMutex.Lock()
	defer c.waitersMutex.Unlock()
	delete(c.waiters, id)
}

func (c *Client) wait(ch chan proto.Reply, timeout time.Duration) (proto.Reply, error) {
	select {
	case data, ok := <-ch:
		if !ok {
			return proto.Reply{}, ErrWaiterClosed
		}
		return data, nil
	case <-time.After(timeout):
		return proto.Reply{}, ErrTimeout
	case <-c.closed:
		return proto.Reply{}, ErrClientClosed
	}
}
