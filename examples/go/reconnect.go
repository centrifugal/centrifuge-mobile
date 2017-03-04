package main

// Demonstrate how to reconnect.

import (
	"fmt"
	"log"
	"time"

	"github.com/centrifugal/centrifuge-mobile"
	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
)

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)
}

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

type eventHandler struct {
	done chan struct{}
}

func (h *eventHandler) OnConnect(c *centrifuge.Client) {
	log.Println("Connected")
	return
}

func (h *eventHandler) OnDisconnect(c *centrifuge.Client) {
	log.Println("Disconnected")
	return
}

type subEventHandler struct{}

func (h *subEventHandler) OnMessage(sub *centrifuge.Sub, msg *centrifuge.Message) error {
	log.Println(fmt.Sprintf("New message received in channel %s: %#v", sub.Channel(), msg))
	return nil
}

func newConnection(done chan struct{}) *centrifuge.Client {
	creds := credentials()
	wsURL := "ws://localhost:8000/connection/websocket"

	handler := &eventHandler{
		done: done,
	}

	events := centrifuge.NewEventHandler()
	events.OnConnect(handler)
	events.OnDisconnect(handler)

	c := centrifuge.New(wsURL, creds, events, centrifuge.DefaultConfig())

	err := c.Connect()
	if err != nil {
		log.Fatalln(err)
	}

	subEvents := centrifuge.NewSubEventHandler()
	subEvents.OnMessage(&subEventHandler{})

	sub, err := c.Subscribe("public:chat", subEvents)
	if err != nil {
		log.Fatalln(err)
	}

	go func() {
		for {
			history, err := sub.History()
			if err != nil {
				log.Printf("Error retreiving channel history: %s", err.Error())
			} else {
				log.Printf("%d messages in channel history", history.NumMessages())
			}
			time.Sleep(time.Second)
		}
	}()

	return c
}

func main() {
	log.Println("Start program")
	done := make(chan struct{})
	c := newConnection(done)
	defer c.Close()
	<-done
}
