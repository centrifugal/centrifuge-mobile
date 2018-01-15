package main

// Connect, subscribe on channel, publish into channel, read presence and history info.

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/centrifugal/centrifuge-mobile"
	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
)

type TestMessage struct {
	Input string `json:"input"`
}

type subEventHandler struct{}

func (h *subEventHandler) OnMessage(sub *centrifuge.Sub, msg *centrifuge.Message) {
	log.Println(fmt.Sprintf("New message received in channel %s: %#v", sub.Channel(), msg))
}

func (h *subEventHandler) OnJoin(sub *centrifuge.Sub, msg *centrifuge.ClientInfo) {
	log.Println(fmt.Sprintf("User %s (client ID %s) joined channel %s", msg.User, msg.Client, sub.Channel()))
}

func (h *subEventHandler) OnLeave(sub *centrifuge.Sub, msg *centrifuge.ClientInfo) {
	log.Println(fmt.Sprintf("User %s (client ID %s) left channel %s", msg.User, msg.Client, sub.Channel()))
}

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

func main() {
	// In production you need to receive credentials from application backend.
	creds := credentials()

	started := time.Now()

	wsURL := "ws://localhost:8000/connection/websocket"
	c := centrifuge.New(wsURL, creds, nil, centrifuge.DefaultConfig())
	defer c.Close()

	err := c.Connect()
	if err != nil {
		log.Fatalln(err)
	}

	events := centrifuge.NewSubEventHandler()
	subEventHandler := &subEventHandler{}
	events.OnMessage(subEventHandler)
	events.OnJoin(subEventHandler)
	events.OnLeave(subEventHandler)

	sub, err := c.Subscribe("public:chat", events)
	if err != nil {
		log.Fatalln(err)
	}

	data := TestMessage{Input: "example input"}
	dataBytes, _ := json.Marshal(data)
	err = sub.Publish(dataBytes)
	if err != nil {
		log.Fatalln(err)
	}

	history, err := sub.History()
	if err != nil {
		log.Fatalln(err)
	}
	log.Printf("%d messages in channel %s history", len(history), sub.Channel())

	presence, err := sub.Presence()
	if err != nil {
		log.Fatalln(err)
	}
	log.Printf("%d clients in channel %s", len(presence), sub.Channel())

	err = sub.Unsubscribe()
	if err != nil {
		log.Fatalln(err)
	}

	log.Printf("%s", time.Since(started))

}
