package main

// Demonstrate how to resque from credentials expiration
// (when connection_lifetime set in Centrifugo).

import (
	"fmt"
	"log"

	"github.com/centrifugal/centrifuge-mobile"
	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
)

// In production you need to receive credentials from application backend.
func credentials() *centrifuge.Credentials {
	// Never show secret to client of your application. Keep it on your application backend only.
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

type eventHandler struct {
	done chan struct{}
}

func (h *eventHandler) OnDisconnect(c *centrifuge.Client, ctx *centrifuge.DisconnectContext) {
	log.Println("Disconnected")
	close(h.done)
}

func (h *eventHandler) OnRefresh(c *centrifuge.Client) (*centrifuge.Credentials, error) {
	log.Println("Refresh")
	return credentials(), nil
}

type subEventHandler struct{}

func (h *subEventHandler) OnMessage(sub *centrifuge.Sub, msg *centrifuge.Message) error {
	log.Println(fmt.Sprintf("New message received in channel %s: %#v", sub.Channel(), msg))
	return nil
}

func newConnection(done chan struct{}) *centrifuge.Client {
	creds := credentials()
	wsURL := "ws://localhost:8000/connection/websocket"

	handler := &eventHandler{done}

	events := centrifuge.NewEventHandler()
	events.OnDisconnect(handler)
	events.OnRefresh(handler)

	c := centrifuge.New(wsURL, creds, events, centrifuge.DefaultConfig())

	err := c.Connect()
	if err != nil {
		log.Fatalln(err)
	}

	subEvents := centrifuge.NewSubEventHandler()
	subEvents.OnMessage(&subEventHandler{})

	_, err = c.Subscribe("public:chat", subEvents)
	if err != nil {
		log.Fatalln(err)
	}

	return c
}

func main() {
	log.Println("Start program")
	done := make(chan struct{})
	c := newConnection(done)
	defer c.Close()
	<-done
}
