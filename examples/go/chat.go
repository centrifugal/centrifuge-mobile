package main

// Simple chat using our public demo on Heroku.
// See and communicate over web version at https://jsfiddle.net/FZambia/yG7Uw/

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/centrifugal/centrifuge-mobile"
	"github.com/centrifugal/centrifugo/libcentrifugo/auth"
	"golang.org/x/crypto/ssh/terminal"
)

type ChatMessage struct {
	Input string `json:"input"`
	Nick  string `json:"nick"`
}

// In production you need to receive credentials from application backend.
func credentials() *centrifuge.Credentials {
	// Never show secret to client of your application. Keep it on your application backend only.
	secret := "secret"
	// Application user ID - anonymous in this case.
	user := ""
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
	term *terminal.Terminal
}

func (h *eventHandler) OnConnect(c *centrifuge.Client, ctx *centrifuge.ConnectContext) {
	log.Printf("Connected to chat")
	return
}

func (h *eventHandler) OnDisconnect(c *centrifuge.Client, ctx *centrifuge.DisconnectContext) {
	log.Printf("Disconnected from chat: %s", ctx.Reason)
	return
}

func (h *eventHandler) OnMessage(sub *centrifuge.Sub, msg *centrifuge.Message) {
	var chatMessage *ChatMessage
	err := json.Unmarshal(msg.Data, &chatMessage)
	if err != nil {
		return
	}
	rePrefix := string(h.term.Escape.Cyan) + chatMessage.Nick + " says:" + string(h.term.Escape.Reset) + "\n"
	fmt.Fprintln(h.term, rePrefix, chatMessage.Input)
}

func (h *eventHandler) OnJoin(sub *centrifuge.Sub, info *centrifuge.ClientInfo) {
	log.Println(fmt.Sprintf("Someone joined: user id %s", info.User))
}

func (h *eventHandler) OnLeave(sub *centrifuge.Sub, info *centrifuge.ClientInfo) {
	log.Println(fmt.Sprintf("Someone left: user id %s", info.User))
}

func (h *eventHandler) OnSubscribeSuccess(sub *centrifuge.Sub, ctx *centrifuge.SubscribeSuccessContext) {
	log.Println(fmt.Sprintf("Subscribed on channel %s", sub.Channel()))
}

func (h *eventHandler) OnUnsubscribe(sub *centrifuge.Sub, ctx *centrifuge.UnsubscribeContext) {
	log.Println(fmt.Sprintf("Unsubscribed from channel %s", sub.Channel()))
}

func run(term *terminal.Terminal) {
	creds := credentials()
	wsURL := "wss://centrifugo.herokuapp.com/connection/websocket"

	handler := &eventHandler{term}
	events := centrifuge.NewEventHandler()
	events.OnConnect(handler)
	events.OnDisconnect(handler)
	c := centrifuge.New(wsURL, creds, events, centrifuge.DefaultConfig())

	subEvents := centrifuge.NewSubEventHandler()
	subEvents.OnMessage(handler)
	subEvents.OnSubscribeSuccess(handler)
	subEvents.OnUnsubscribe(handler)
	subEvents.OnJoin(handler)
	subEvents.OnLeave(handler)

	sub, err := c.Subscribe("jsfiddle-chat", subEvents)
	if err != nil {
		log.Fatalln(err)
	}

	err = c.Connect()
	if err != nil {
		log.Fatalln(err)
	}

	chat(sub, term)
}

func chat(sub *centrifuge.Sub, term *terminal.Terminal) error {
	for {
		line, err := term.ReadLine()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if line == "" {
			continue
		}
		msg := &ChatMessage{
			Input: line,
			Nick:  "goexample",
		}
		data, err := json.Marshal(msg)
		sub.Publish(data)
	}
}

func main() {
	if !terminal.IsTerminal(0) || !terminal.IsTerminal(1) {
		fmt.Errorf("stdin/stdout should be terminal")
		os.Exit(1)
	}
	oldState, err := terminal.MakeRaw(0)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	defer terminal.Restore(0, oldState)
	screen := struct {
		io.Reader
		io.Writer
	}{os.Stdin, os.Stdout}
	term := terminal.NewTerminal(screen, "")
	term.SetPrompt(string(term.Escape.Red) + "> " + string(term.Escape.Reset))

	run(term)
	select {}
}
