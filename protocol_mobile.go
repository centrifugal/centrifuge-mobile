// +build mobile

package centrifuge

import (
	"encoding/json"
)

type ClientInfo struct {
	User        string `json:"user"`
	Client      string `json:"client"`
	DefaultInfo string `json:"default_info,omitempty"`
	ChannelInfo string `json:"channel_info,omitempty"`
}

func clientInfoFromRaw(i *rawClientInfo) *ClientInfo {
	if i == nil {
		return nil
	}

	var defaultInfo string
	var channelInfo string

	if i.DefaultInfo != nil {
		defaultInfo = string(i.DefaultInfo)
	}
	if i.ChannelInfo != nil {
		channelInfo = string(i.ChannelInfo)
	}

	return &ClientInfo{
		User:        i.User,
		Client:      i.Client,
		DefaultInfo: defaultInfo,
		ChannelInfo: channelInfo,
	}
}

type rawClientInfo struct {
	User        string          `json:"user"`
	Client      string          `json:"client"`
	DefaultInfo json.RawMessage `json:"default_info,omitempty"`
	ChannelInfo json.RawMessage `json:"channel_info,omitempty"`
}

type Message struct {
	UID     string      `json:"uid"`
	Info    *ClientInfo `json:"info,omitempty"`
	Channel string      `json:"channel"`
	Data    string      `json:"data"`
	Client  string      `json:"client,omitempty"`
}

func messageFromRaw(m *rawMessage) *Message {
	var data string
	if m.Data != nil {
		data = string(m.Data)
	}
	return &Message{
		UID:     m.UID,
		Channel: m.Channel,
		Info:    clientInfoFromRaw(m.Info),
		Data:    data,
		Client:  m.Client,
	}
}

type rawMessage struct {
	UID     string          `json:"uid"`
	Info    *rawClientInfo  `json:"info,omitempty"`
	Channel string          `json:"channel"`
	Data    json.RawMessage `json:"data"`
	Client  string          `json:"client,omitempty"`
}
