package main

// Simple chat using our public demo on Heroku.
// See and communicate over web version at https://jsfiddle.net/FZambia/yG7Uw/

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/centrifugal/centrifuge-mobile"
)

// ChatMessage is chat app specific message struct.
type ChatMessage struct {
	Input string `json:"input"`
	Nick  string `json:"nick"`
}

// // In production you need to receive credentials from application backend.
// func credentials() *centrifuge.Credentials {
// 	// Never show secret to client of your application. Keep it on your application backend only.
// 	// secret := "secret"
// 	// Application user ID - anonymous in this case.
// 	user := ""
// 	// Exp timestamp as string.
// 	exp := centrifuge.Exp(60)
// 	// Empty info.
// 	info := ""
// 	// Generate client sign so Centrifuge server can trust connection parameters received from client.
// 	sign := ""

// 	return &centrifuge.Credentials{
// 		User: user,
// 		Exp:  exp,
// 		Info: info,
// 		Sign: sign,
// 	}
// }

type eventHandler struct {
	out io.Writer
}

func (h *eventHandler) OnConnect(c *centrifuge.Client, ctx *centrifuge.ConnectContext) {
	fmt.Fprintln(h.out, fmt.Sprintf("Connected to chat with ID %s", ctx.ClientID))
	return
}

func (h *eventHandler) OnDisconnect(c *centrifuge.Client, ctx *centrifuge.DisconnectContext) {
	fmt.Fprintln(h.out, fmt.Sprintf("Disconnected from chat: %s", ctx.Reason))
	return
}

func (h *eventHandler) OnMessage(sub *centrifuge.Sub, pub centrifuge.Pub) {
	var chatMessage *ChatMessage
	err := json.Unmarshal(pub.Data, &chatMessage)
	if err != nil {
		return
	}
	rePrefix := chatMessage.Nick + " says:"
	fmt.Fprintln(h.out, rePrefix, chatMessage.Input)
}

func (h *eventHandler) OnJoin(sub *centrifuge.Sub, info centrifuge.ClientInfo) {
	fmt.Fprintln(h.out, fmt.Sprintf("Someone joined: user id %s, client id %s", info.User, info.Client))
}

func (h *eventHandler) OnLeave(sub *centrifuge.Sub, info centrifuge.ClientInfo) {
	fmt.Fprintln(h.out, fmt.Sprintf("Someone left: user id %s, client id %s", info.User, info.Client))
}

func (h *eventHandler) OnSubscribeSuccess(sub *centrifuge.Sub, ctx centrifuge.SubscribeSuccessContext) {
	fmt.Fprintln(h.out, fmt.Sprintf("Subscribed on channel %s", sub.Channel()))
}

func (h *eventHandler) OnUnsubscribe(sub *centrifuge.Sub, ctx centrifuge.UnsubscribeContext) {
	fmt.Fprintln(h.out, fmt.Sprintf("Unsubscribed from channel %s", sub.Channel()))
}

func main() {
	//creds := credentials()
	wsURL := "ws://localhost:8000/connection/websocket"

	handler := &eventHandler{os.Stdout}

	events := centrifuge.NewEventHandler()
	events.OnConnect(handler)
	events.OnDisconnect(handler)
	c := centrifuge.New(wsURL, events, centrifuge.DefaultConfig())

	subEvents := centrifuge.NewSubEventHandler()
	subEvents.OnMessage(handler)
	subEvents.OnSubscribeSuccess(handler)
	subEvents.OnUnsubscribe(handler)
	subEvents.OnJoin(handler)
	subEvents.OnLeave(handler)

	fmt.Fprintf(os.Stdout, "You can communicate with web version at https://jsfiddle.net/FZambia/yG7Uw/\n")
	fmt.Fprintf(os.Stdout, "Connect to %s\n", wsURL)
	fmt.Fprintf(os.Stdout, "Print something and press ENTER to send\n")

	sub := c.Subscribe("chat:index", subEvents)

	err := c.Connect()
	if err != nil {
		log.Fatalln(err)
	}

	// Read input from stdin.
	go func(sub *centrifuge.Sub) {
		reader := bufio.NewReader(os.Stdin)
		for {
			text, _ := reader.ReadString('\n')
			msg := &ChatMessage{
				Input: text,
				Nick:  "goexample",
			}
			data, _ := json.Marshal(msg)
			sub.Publish(data)
		}
	}(sub)

	// Run until CTRL+C.
	select {}
}
