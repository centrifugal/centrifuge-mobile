package centrifuge

import (
	"time"

	gocentrifuge "github.com/centrifugal/centrifuge-go"
)

// Client to connect to Centrifuge-based server or Centrifugo.
type Client struct {
	client *gocentrifuge.Client
}

// Config defaults.
const (
	DefaultReadTimeoutMilliseconds  = 5000
	DefaultWriteTimeoutMilliseconds = 5000
	DefaultPingIntervalMilliseconds = 25000
	DefaultPrivateChannelPrefix     = "$"
)

// Config contains various client options.
type Config struct {
	ReadTimeoutMilliseconds  int
	WriteTimeoutMilliseconds int
	PingIntervalMilliseconds int
	PrivateChannelPrefix     string
}

// DefaultConfig returns Config with default options.
func DefaultConfig() *Config {
	return &Config{
		PingIntervalMilliseconds: DefaultPingIntervalMilliseconds,
		ReadTimeoutMilliseconds:  DefaultReadTimeoutMilliseconds,
		WriteTimeoutMilliseconds: DefaultWriteTimeoutMilliseconds,
		PrivateChannelPrefix:     DefaultPrivateChannelPrefix,
	}
}

// New initializes Client.
func New(u string, events *EventHub, config *Config) *Client {
	c := gocentrifuge.Config{
		ReadTimeout:          time.Duration(config.ReadTimeoutMilliseconds) * time.Millisecond,
		WriteTimeout:         time.Duration(config.WriteTimeoutMilliseconds) * time.Millisecond,
		PingInterval:         time.Duration(config.PingIntervalMilliseconds) * time.Millisecond,
		PrivateChannelPrefix: config.PrivateChannelPrefix,
	}
	client := &Client{
		client: gocentrifuge.New(u, events.eventHub, c),
	}
	events.setClient(client)
	return client
}

// SetToken allows to set connection token so client
// authenticate itself on connect.
func (c *Client) SetToken(token string) {
	c.client.SetToken(token)
}

// SetConnectData allows to set data to send in connect message.
func (c *Client) SetConnectData(data []byte) {
	c.client.SetConnectData(data)
}

// SetHeader allows to set custom header sent in Upgrade HTTP request.
func (c *Client) SetHeader(key, value string) {
	c.client.SetHeader(key, value)
}

// Send data to server asynchronously.
func (c *Client) Send(data []byte) error {
	return c.client.Send(data)
}

// RPC allows to make RPC – send data to server ant wait for response.
// RPC handler must be registered on server.
func (c *Client) RPC(data []byte) ([]byte, error) {
	return c.client.RPC(data)
}

// Close closes Client connection and cleans ups everything.
func (c *Client) Close() error {
	return c.client.Close()
}

// Connect dials to server and sends connect message.
func (c *Client) Connect() error {
	return c.client.Connect()
}

// Disconnect client from server.
func (c *Client) Disconnect() error {
	return c.client.Disconnect()
}

// Publish data into channel.
func (c *Client) Publish(channel string, data []byte) error {
	return c.client.Publish(channel, data)
}

// Subscribe allows to subscribe on channel.
func (c *Client) Subscribe(channel string, events *SubscriptionEventHub) (*Subscription, error) {
	sub, err := c.client.Subscribe(channel, events.subEventHub)
	if err != nil {
		return nil, err
	}
	return &Subscription{
		sub: sub,
	}, nil
}

// SubscribeSync allows to subscribe on channel and wait until subscribe success or error.
func (c *Client) SubscribeSync(channel string, events *SubscriptionEventHub) (*Subscription, error) {
	sub, err := c.client.SubscribeSync(channel, events.subEventHub)
	if err != nil {
		return nil, err
	}
	return &Subscription{
		sub: sub,
	}, nil
}
