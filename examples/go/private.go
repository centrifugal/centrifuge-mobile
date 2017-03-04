package main

// Private subscription example.

import (
	"fmt"
	"log"
	"os"

	"github.com/centrifugal/centrifuge-mobile"
	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
)

func credentials() *centrifuge.Credentials {
	secret := "secret"

	// Application user ID.
	user := "42"

	// Current timestamp as string.
	timestamp := centrifuge.Timestamp()

	// Empty info.
	info := ""

	// Generate client token so Centrifugo server can trust connection parameters received from client.
	token := auth.GenerateClientToken(secret, user, timestamp, info)

	return &centrifuge.Credentials{
		User:      user,
		Timestamp: timestamp,
		Info:      info,
		Token:     token,
	}
}

type eventHandler struct{}

func (h *eventHandler) OnPrivateSub(c *centrifuge.Client, req *centrifuge.PrivateRequest) (*centrifuge.PrivateSign, error) {
	// Here we allow everyone to subscribe on private channel.
	// To reject subscription we could return any error from this func.
	// In most real application secret key must not be kept on client side
	// and here must be request to your backend to get channel sign.
	info := ""
	sign := auth.GenerateChannelSign("secret", req.ClientID, req.Channel, info)
	privateSign := &centrifuge.PrivateSign{Sign: sign, Info: info}
	return privateSign, nil
}

type subEventHandler struct{}

func (h *subEventHandler) OnSubscribeSuccess(sub *centrifuge.Sub) {
	log.Println(fmt.Sprintf("Successfully subscribed on private channel %s", sub.Channel()))
	os.Exit(0)
}

func (h *subEventHandler) OnSubscribeError(sub *centrifuge.Sub, err error) {
	log.Println(fmt.Sprintf("Error subscribing to private channel %s: %v", sub.Channel(), err))
	os.Exit(1)
}

func newConnection() *centrifuge.Client {
	creds := credentials()
	wsURL := "ws://localhost:8000/connection/websocket"

	handler := &eventHandler{}
	events := centrifuge.NewEventHandler()
	events.OnPrivateSub(handler)

	c := centrifuge.New(wsURL, creds, events, centrifuge.DefaultConfig())

	err := c.Connect()
	if err != nil {
		log.Fatalln(err)
	}
	return c
}

func main() {
	log.Println("Start program")
	c := newConnection()
	defer c.Close()

	events := centrifuge.NewSubEventHandler()
	subEventHandler := &subEventHandler{}
	events.OnSubscribeSuccess(subEventHandler)
	events.OnSubscribeError(subEventHandler)

	// Subscribe on private channel.
	_, err := c.Subscribe("$public:chat", events)
	if err != nil {
		log.Fatalln(err)
	}

	select {}
}
