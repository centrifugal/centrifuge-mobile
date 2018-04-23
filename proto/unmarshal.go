package proto

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/gogo/protobuf/proto"
)

// UnmarshalJSON helps to unmarshal comamnd method when set as string.
func (m *MethodType) UnmarshalJSON(data []byte) error {
	val, err := strconv.Atoi(string(data))
	if err != nil {
		method := strings.Trim(strings.ToUpper(string(data)), `"`)
		if v, ok := MethodType_value[method]; ok {
			*m = MethodType(v)
			return nil
		}
		return err
	}
	*m = MethodType(val)
	return nil
}

// MessageDecoder ...
type MessageDecoder interface {
	Decode([]byte) (*Message, error)
	DecodePub([]byte) (*Pub, error)
	DecodeJoin([]byte) (*Join, error)
	DecodeLeave([]byte) (*Leave, error)
	DecodePush([]byte) (*Push, error)
	DecodeUnsub([]byte) (*Unsub, error)
}

// JSONMessageDecoder ...
type JSONMessageDecoder struct {
}

// NewJSONMessageDecoder ...
func NewJSONMessageDecoder() *JSONMessageDecoder {
	return &JSONMessageDecoder{}
}

// Decode ...
func (e *JSONMessageDecoder) Decode(data []byte) (*Message, error) {
	var m Message
	err := json.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DecodePub ...
func (e *JSONMessageDecoder) DecodePub(data []byte) (*Pub, error) {
	var m Pub
	err := json.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DecodeJoin ...
func (e *JSONMessageDecoder) DecodeJoin(data []byte) (*Join, error) {
	var m Join
	err := json.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DecodeLeave  ...
func (e *JSONMessageDecoder) DecodeLeave(data []byte) (*Leave, error) {
	var m Leave
	err := json.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DecodePush ...
func (e *JSONMessageDecoder) DecodePush(data []byte) (*Push, error) {
	var m Push
	err := json.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DecodeUnsub ...
func (e *JSONMessageDecoder) DecodeUnsub(data []byte) (*Unsub, error) {
	var m Unsub
	err := json.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ProtobufMessageDecoder ...
type ProtobufMessageDecoder struct {
}

// NewProtobufMessageDecoder ...
func NewProtobufMessageDecoder() *ProtobufMessageDecoder {
	return &ProtobufMessageDecoder{}
}

// Decode ...
func (e *ProtobufMessageDecoder) Decode(data []byte) (*Message, error) {
	var m Message
	err := m.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DecodePub ...
func (e *ProtobufMessageDecoder) DecodePub(data []byte) (*Pub, error) {
	var m Pub
	err := m.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DecodeJoin ...
func (e *ProtobufMessageDecoder) DecodeJoin(data []byte) (*Join, error) {
	var m Join
	err := m.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DecodeLeave  ...
func (e *ProtobufMessageDecoder) DecodeLeave(data []byte) (*Leave, error) {
	var m Leave
	err := m.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DecodePush ...
func (e *ProtobufMessageDecoder) DecodePush(data []byte) (*Push, error) {
	var m Push
	err := m.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// DecodeUnsub ...
func (e *ProtobufMessageDecoder) DecodeUnsub(data []byte) (*Unsub, error) {
	var m Unsub
	err := m.Unmarshal(data)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ReplyDecoder ...
type ReplyDecoder interface {
	Reset([]byte) error
	Decode() (*Reply, error)
}

// JSONReplyDecoder ...
type JSONReplyDecoder struct {
	decoder *json.Decoder
}

// NewJSONReplyDecoder ...
func NewJSONReplyDecoder(data []byte) *JSONReplyDecoder {
	return &JSONReplyDecoder{
		decoder: json.NewDecoder(bytes.NewReader(data)),
	}
}

// Reset ...
func (d *JSONReplyDecoder) Reset(data []byte) error {
	d.decoder = json.NewDecoder(bytes.NewReader(data))
	return nil
}

// Decode ...
func (d *JSONReplyDecoder) Decode() (*Reply, error) {
	var c Reply
	err := d.decoder.Decode(&c)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ProtobufReplyDecoder ...
type ProtobufReplyDecoder struct {
	data   []byte
	offset int
}

// NewProtobufReplyDecoder ...
func NewProtobufReplyDecoder(data []byte) *ProtobufReplyDecoder {
	return &ProtobufReplyDecoder{
		data: data,
	}
}

// Reset ...
func (d *ProtobufReplyDecoder) Reset(data []byte) error {
	d.data = data
	d.offset = 0
	return nil
}

// Decode ...
func (d *ProtobufReplyDecoder) Decode() (*Reply, error) {
	if d.offset < len(d.data) {
		var c Reply
		l, n := binary.Uvarint(d.data[d.offset:])
		replyBytes := d.data[d.offset+n : d.offset+n+int(l)]
		err := c.Unmarshal(replyBytes)
		if err != nil {
			return nil, err
		}
		d.offset = d.offset + n + int(l)
		return &c, nil
	}
	return nil, io.EOF
}

// ResultDecoder ...
type ResultDecoder interface {
	Decode([]byte, interface{}) error
}

// JSONResultDecoder ...
type JSONResultDecoder struct{}

// NewJSONResultDecoder ...
func NewJSONResultDecoder() *JSONResultDecoder {
	return &JSONResultDecoder{}
}

// Decode ...
func (e *JSONResultDecoder) Decode(data []byte, dest interface{}) error {
	return json.Unmarshal(data, dest)
}

// ProtobufResultDecoder ...
type ProtobufResultDecoder struct{}

// NewProtobufResultDecoder ...
func NewProtobufResultDecoder() *ProtobufResultDecoder {
	return &ProtobufResultDecoder{}
}

// Decode ...
func (e *ProtobufResultDecoder) Decode(data []byte, dest interface{}) error {
	m, ok := dest.(proto.Unmarshaler)
	if !ok {
		return fmt.Errorf("can not unmarshal type from Protobuf")
	}
	return m.Unmarshal(data)
}
