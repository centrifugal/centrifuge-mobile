package proto

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/gogo/protobuf/proto"
)

// CommandEncoder ...
type CommandEncoder interface {
	Encode(cmd *Command) ([]byte, error)
}

// JSONCommandEncoder ...
type JSONCommandEncoder struct {
}

// NewJSONCommandEncoder ...
func NewJSONCommandEncoder() *JSONCommandEncoder {
	return &JSONCommandEncoder{}
}

// Encode ...
func (e *JSONCommandEncoder) Encode(cmd *Command) ([]byte, error) {
	return json.Marshal(cmd)
}

// ProtobufCommandEncoder ...
type ProtobufCommandEncoder struct {
}

// NewProtobufCommandEncoder ...
func NewProtobufCommandEncoder() *ProtobufCommandEncoder {
	return &ProtobufCommandEncoder{}
}

// Encode ...
func (e *ProtobufCommandEncoder) Encode(cmd *Command) ([]byte, error) {
	commandBytes, err := cmd.Marshal()
	if err != nil {
		return nil, err
	}
	bs := make([]byte, 8)
	n := binary.PutUvarint(bs, uint64(len(commandBytes)))
	var buf bytes.Buffer
	buf.Write(bs[:n])
	buf.Write(commandBytes)
	return buf.Bytes(), nil
}

// MessageEncoder ...
type MessageEncoder interface {
	Encode(*Message) ([]byte, error)
	EncodePush(*Push) ([]byte, error)
	EncodePub(*Pub) ([]byte, error)
	EncodeJoin(*Join) ([]byte, error)
	EncodeLeave(*Leave) ([]byte, error)
	EncodeUnsub(*Unsub) ([]byte, error)
}

// JSONMessageEncoder ...
type JSONMessageEncoder struct {
}

// NewJSONMessageEncoder ...
func NewJSONMessageEncoder() *JSONMessageEncoder {
	return &JSONMessageEncoder{}
}

// Encode ...
func (e *JSONMessageEncoder) Encode(message *Message) ([]byte, error) {
	return json.Marshal(message)
}

// EncodePub ...
func (e *JSONMessageEncoder) EncodePub(message *Pub) ([]byte, error) {
	return json.Marshal(message)
}

// EncodePush ...
func (e *JSONMessageEncoder) EncodePush(message *Push) ([]byte, error) {
	return json.Marshal(message)
}

// EncodeJoin ...
func (e *JSONMessageEncoder) EncodeJoin(message *Join) ([]byte, error) {
	return json.Marshal(message)
}

// EncodeLeave ...
func (e *JSONMessageEncoder) EncodeLeave(message *Leave) ([]byte, error) {
	return json.Marshal(message)
}

// EncodeUnsub ...
func (e *JSONMessageEncoder) EncodeUnsub(message *Unsub) ([]byte, error) {
	return json.Marshal(message)
}

// ProtobufMessageEncoder ...
type ProtobufMessageEncoder struct {
}

// NewProtobufMessageEncoder ...
func NewProtobufMessageEncoder() *ProtobufMessageEncoder {
	return &ProtobufMessageEncoder{}
}

// Encode ...
func (e *ProtobufMessageEncoder) Encode(message *Message) ([]byte, error) {
	return message.Marshal()
}

// EncodePub ...
func (e *ProtobufMessageEncoder) EncodePub(message *Pub) ([]byte, error) {
	return message.Marshal()
}

// EncodePush ...
func (e *ProtobufMessageEncoder) EncodePush(message *Push) ([]byte, error) {
	return message.Marshal()
}

// EncodeJoin ...
func (e *ProtobufMessageEncoder) EncodeJoin(message *Join) ([]byte, error) {
	return message.Marshal()
}

// EncodeLeave ...
func (e *ProtobufMessageEncoder) EncodeLeave(message *Leave) ([]byte, error) {
	return message.Marshal()
}

// EncodeUnsub ...
func (e *ProtobufMessageEncoder) EncodeUnsub(message *Unsub) ([]byte, error) {
	return message.Marshal()
}

// ReplyEncoder ...
type ReplyEncoder interface {
	Reset()
	Encode(*Reply) error
	Finish() []byte
}

// JSONReplyEncoder ...
type JSONReplyEncoder struct {
	buffer bytes.Buffer
}

// NewJSONReplyEncoder ...
func NewJSONReplyEncoder() *JSONReplyEncoder {
	return &JSONReplyEncoder{}
}

// Reset ...
func (e *JSONReplyEncoder) Reset() {
	e.buffer.Reset()
}

// Encode ...
func (e *JSONReplyEncoder) Encode(r *Reply) error {
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	e.buffer.Write(data)
	e.buffer.WriteString("\n")
	return nil
}

// Finish ...
func (e *JSONReplyEncoder) Finish() []byte {
	data := e.buffer.Bytes()
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	return dataCopy
}

// ProtobufReplyEncoder ...
type ProtobufReplyEncoder struct {
	buffer bytes.Buffer
}

// NewProtobufReplyEncoder ...
func NewProtobufReplyEncoder() *ProtobufReplyEncoder {
	return &ProtobufReplyEncoder{}
}

// Encode ...
func (e *ProtobufReplyEncoder) Encode(r *Reply) error {
	replyBytes, err := r.Marshal()
	if err != nil {
		return err
	}
	bs := make([]byte, 8)
	n := binary.PutUvarint(bs, uint64(len(replyBytes)))
	e.buffer.Write(bs[:n])
	e.buffer.Write(replyBytes)
	return nil
}

// Reset ...
func (e *ProtobufReplyEncoder) Reset() {
	e.buffer.Reset()
}

// Finish ...
func (e *ProtobufReplyEncoder) Finish() []byte {
	data := e.buffer.Bytes()
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	return dataCopy
}

// ParamsEncoder ...
type ParamsEncoder interface {
	Encode(request interface{}) ([]byte, error)
}

// JSONParamsEncoder ...
type JSONParamsEncoder struct{}

// NewJSONParamsEncoder ...
func NewJSONParamsEncoder() *JSONParamsEncoder {
	return &JSONParamsEncoder{}
}

// Encode ...
func (d *JSONParamsEncoder) Encode(r interface{}) ([]byte, error) {
	return json.Marshal(r)
}

// ProtobufParamsEncoder ...
type ProtobufParamsEncoder struct{}

// NewProtobufParamsEncoder ...
func NewProtobufParamsEncoder() *ProtobufParamsEncoder {
	return &ProtobufParamsEncoder{}
}

// Encode ...
func (d *ProtobufParamsEncoder) Encode(r interface{}) ([]byte, error) {
	m, ok := r.(proto.Marshaler)
	if !ok {
		return nil, fmt.Errorf("can not marshal type to Protobuf")
	}
	return m.Marshal()
}
