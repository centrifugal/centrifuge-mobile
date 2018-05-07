package main

// Connect, subscribe on channel, publish into channel, read presence and history info.

import (
	"encoding/json"
	"log"
	"time"

	"github.com/centrifugal/centrifuge-mobile"
)

// // In production you need to receive credentials from application backend.
// func credentials() *centrifuge.Credentials {
// 	// Never show secret to client of your application. Keep it on your application backend only.
// 	secret := "secret"
// 	// Application user ID.
// 	user := "42"
// 	// Exp as string.
// 	exp := centrifuge.Exp(60)
// 	// Empty info.
// 	info := ""
// 	// Generate sign so Centrifugo server can trust connection parameters received from client.
// 	sign := centrifuge.GenerateClientSign(secret, user, exp, info)

// 	return &centrifuge.Credentials{
// 		User: user,
// 		Exp:  exp,
// 		Info: info,
// 		Sign: sign,
// 	}
// }

type testMessage struct {
	Input string `json:"input"`
}

type eventHandler struct{}

func (h *eventHandler) OnConnect(c *centrifuge.Client, e *centrifuge.ConnectEvent) {
	log.Printf("Connected to chat with ID %s", e.ClientID)
}

func (h *eventHandler) OnError(c *centrifuge.Client, e *centrifuge.ErrorEvent) {
	log.Printf("Error: %s", e.Message)
}

func (h *eventHandler) OnDisconnect(c *centrifuge.Client, e *centrifuge.DisconnectEvent) {
	log.Printf("Disconnected from chat: %s", e.Reason)
}

type subEventHandler struct{}

func (h *subEventHandler) OnPublish(sub *centrifuge.Subscription, e *centrifuge.PublishEvent) {
	log.Printf("New message received in channel %s: %#v", sub.Channel(), e.Data)
}

func (h *subEventHandler) OnJoin(sub *centrifuge.Subscription, e *centrifuge.JoinEvent) {
	log.Printf("User %s (client ID %s) joined channel %s", e.User, e.Client, sub.Channel())
}

func (h *subEventHandler) OnLeave(sub *centrifuge.Subscription, e *centrifuge.LeaveEvent) {
	log.Printf("User %s (client ID %s) left channel %s", e.User, e.Client, sub.Channel())
}

func (h *subEventHandler) OnSubscribeSuccess(sub *centrifuge.Subscription, e *centrifuge.SubscribeSuccessEvent) {
	log.Printf("Successfully subscribed on channel %s", sub.Channel())
}

func (h *subEventHandler) OnSubscribeError(sub *centrifuge.Subscription, e *centrifuge.SubscribeErrorEvent) {
	log.Printf("Error subscribing on channel %s: %v", sub.Channel(), e.Error)
}

func main() {
	// In production you need to receive credentials from application backend.
	// creds := credentials()

	started := time.Now()

	wsURL := "ws://localhost:8000/connection/websocket"
	events := centrifuge.NewEventHub()
	eventHandler := &eventHandler{}
	events.OnConnect(eventHandler)
	events.OnDisconnect(eventHandler)
	events.OnError(eventHandler)
	c := centrifuge.New(wsURL, events, centrifuge.DefaultConfig())

	err := c.Connect()
	if err != nil {
		log.Fatalln(err)
	}

	subEvents := centrifuge.NewSubscriptionEventHub()
	subEventHandler := &subEventHandler{}
	subEvents.OnPublish(subEventHandler)
	subEvents.OnJoin(subEventHandler)
	subEvents.OnLeave(subEventHandler)
	subEvents.OnSubscribeSuccess(subEventHandler)
	subEvents.OnSubscribeError(subEventHandler)

	sub, err := c.Subscribe("index", subEvents)
	if err != nil {
		log.Fatalln(err)
	}

	data := testMessage{Input: "example input"}
	dataBytes, _ := json.Marshal(data)
	err = sub.Publish(dataBytes)
	if err != nil {
		log.Fatalln(err)
	}

	history, err := sub.History()
	if err != nil {
		log.Fatalln(err)
	}
	log.Printf("%d messages in channel %s history", history.NumItems(), sub.Channel())

	presence, err := sub.Presence()
	if err != nil {
		log.Fatalln(err)
	}
	log.Printf("%d clients in channel %s", presence.NumItems(), sub.Channel())

	err = sub.Unsubscribe()
	if err != nil {
		log.Fatalln(err)
	}

	log.Printf("%s", time.Since(started))

}
