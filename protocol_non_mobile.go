// +build !mobile

package centrifuge

import (
	"encoding/json"
)

type ClientInfo struct {
	User        string          `json:"user"`
	Client      string          `json:"client"`
	DefaultInfo json.RawMessage `json:"default_info,omitempty"`
	ChannelInfo json.RawMessage `json:"channel_info,omitempty"`
}

func clientInfoFromRaw(i *rawClientInfo) *ClientInfo {
	info := ClientInfo(*i)
	return &info
}

type rawClientInfo struct {
	User        string          `json:"user"`
	Client      string          `json:"client"`
	DefaultInfo json.RawMessage `json:"default_info,omitempty"`
	ChannelInfo json.RawMessage `json:"channel_info,omitempty"`
}

type Message struct {
	UID     string          `json:"uid"`
	Info    *ClientInfo     `json:"info,omitempty"`
	Channel string          `json:"channel"`
	Data    json.RawMessage `json:"data"`
	Client  string          `json:"client,omitempty"`
}

func messageFromRaw(m *rawMessage) *Message {
	msg := Message(*m)
	return &msg
}

type rawMessage struct {
	UID     string          `json:"uid"`
	Info    *ClientInfo     `json:"info,omitempty"`
	Channel string          `json:"channel"`
	Data    json.RawMessage `json:"data"`
	Client  string          `json:"client,omitempty"`
}
