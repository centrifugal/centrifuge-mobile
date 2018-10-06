package main

// Connect, subscribe on channel, publish into channel, read presence and history info.

import (
	"encoding/json"
	"fmt"
	"log"

	centrifuge "github.com/centrifugal/centrifuge-mobile"
)

type testMessage struct {
	Input string `json:"input"`
}

type eventHandler struct{}

func (h *eventHandler) OnConnect(c *centrifuge.Client, e *centrifuge.ConnectEvent) {
	log.Printf("client connected")
}

func (h *eventHandler) OnDisconnect(c *centrifuge.Client, e *centrifuge.DisconnectEvent) {
	log.Println("client diconnected")
}

type subEventHandler struct{}

func (h *subEventHandler) OnPublish(sub *centrifuge.Subscription, e *centrifuge.PublishEvent) {
	log.Println(fmt.Sprintf("New publication received from channel %s: %s", sub.Channel(), string(e.Data)))
}

func (h *subEventHandler) OnJoin(sub *centrifuge.Subscription, e *centrifuge.JoinEvent) {
	log.Println(fmt.Sprintf("User %s (client ID %s) joined channel %s", e.User, e.Client, sub.Channel()))
}

func (h *subEventHandler) OnLeave(sub *centrifuge.Subscription, e *centrifuge.LeaveEvent) {
	log.Println(fmt.Sprintf("User %s (client ID %s) left channel %s", e.User, e.Client, sub.Channel()))
}

func main() {
	url := "ws://localhost:8000/connection/websocket"

	c := centrifuge.New(url, centrifuge.DefaultConfig())
	defer c.Close()

	eventHandler := &eventHandler{}
	c.OnConnect(eventHandler)
	c.OnDisconnect(eventHandler)

	err := c.Connect()
	if err != nil {
		log.Fatalln(err)
	}

	sub, err := c.NewSubscription("chat:index")
	if err != nil {
		log.Fatalln(err)
	}

	subEventHandler := &subEventHandler{}
	sub.OnPublish(subEventHandler)
	sub.OnJoin(subEventHandler)
	sub.OnLeave(subEventHandler)

	err = sub.Subscribe()
	if err != nil {
		log.Fatalln(err)
	}

	data := testMessage{Input: "example input"}
	dataBytes, _ := json.Marshal(data)
	err = sub.Publish(dataBytes)
	if err != nil {
		log.Fatalln(err)
	}
}
