package centrifuge

import (
	"encoding/json"
)

type clientCommand struct {
	ID     int32  `json:"id"`
	Method string `json:"method"`
}

type connectClientCommand struct {
	clientCommand
	Params connectParams `json:"params"`
}

type refreshClientCommand struct {
	clientCommand
	Params refreshParams `json:"params"`
}

type subscribeClientCommand struct {
	clientCommand
	Params subscribeParams `json:"params"`
}

type unsubscribeClientCommand struct {
	clientCommand
	Params unsubscribeParams `json:"params"`
}

type publishClientCommand struct {
	clientCommand
	Params publishParams `json:"params"`
}

type presenceClientCommand struct {
	clientCommand
	Params presenceParams `json:"params"`
}

type historyClientCommand struct {
	clientCommand
	Params historyParams `json:"params"`
}

type pingClientCommand struct {
	clientCommand
}

type connectParams struct {
	User string `json:"user"`
	Exp  string `json:"exp"`
	Info string `json:"info"`
	Sign string `json:"sign"`
}

type refreshParams struct {
	User string `json:"user"`
	Exp  string `json:"exp"`
	Info string `json:"info"`
	Sign string `json:"sign"`
}

type subscribeParams struct {
	Channel string `json:"channel"`
	Client  string `json:"client,omitempty"`
	Last    string `json:"last,omitempty"`
	Recover bool   `json:"recover,omitempty"`
	Info    string `json:"info,omitempty"`
	Sign    string `json:"sign,omitempty"`
}

type unsubscribeParams struct {
	Channel string `json:"channel"`
}

type publishParams struct {
	Channel string           `json:"channel"`
	Data    *json.RawMessage `json:"data"`
}

type presenceParams struct {
	Channel string `json:"channel"`
}

type historyParams struct {
	Channel string `json:"channel"`
}

type response struct {
	ID     int32           `json:"id,omitempty"`
	Error  string          `json:"error"`
	Result json.RawMessage `json:"result"`
}

type joinLeaveMessage struct {
	Channel string        `json:"channel"`
	Data    rawClientInfo `json:"data"`
}

type connectResponseBody struct {
	Version string `json:"version"`
	Client  string `json:"client"`
	Expires bool   `json:"expires"`
	Expired bool   `json:"expired"`
	TTL     int64  `json:"ttl"`
}

type subscribeResponseBody struct {
	Channel   string       `json:"channel"`
	Status    bool         `json:"status"`
	Last      string       `json:"last"`
	Messages  []rawMessage `json:"messages"`
	Recovered bool         `json:"recovered"`
}

type unsubscribeResponseBody struct {
	Channel string `json:"channel"`
	Status  bool   `json:"status"`
}

type publishResponseBody struct {
	Channel string `json:"channel"`
	Status  bool   `json:"status"`
}

type presenceResponseBody struct {
	Channel string                   `json:"channel"`
	Data    map[string]rawClientInfo `json:"data"`
}

type historyResponseBody struct {
	Channel string       `json:"channel"`
	Data    []rawMessage `json:"data"`
}

type disconnectAdvice struct {
	Reason    string `json:"reason"`
	Reconnect bool   `json:"reconnect"`
}

var (
	arrayJsonPrefix  byte = '['
	objectJsonPrefix byte = '{'
)

func responsesFromClientMsg(msg []byte) ([]response, error) {
	var resps []response
	firstByte := msg[0]
	switch firstByte {
	case objectJsonPrefix:
		// single command request
		var resp response
		err := json.Unmarshal(msg, &resp)
		if err != nil {
			return nil, err
		}
		resps = append(resps, resp)
	case arrayJsonPrefix:
		// array of commands received
		err := json.Unmarshal(msg, &resps)
		if err != nil {
			return nil, err
		}
	default:
		return nil, ErrInvalidMessage
	}
	return resps, nil
}
