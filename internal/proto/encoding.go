package proto

import "sync"

// Encoding determines connection protocol encoding in use.
type Encoding int

const (
	// EncodingJSON means JSON protocol.
	EncodingJSON Encoding = 0
	// EncodingProtobuf means protobuf protocol.
	EncodingProtobuf Encoding = 1
)

// GetMessageEncoder ...
func GetMessageEncoder(enc Encoding) MessageEncoder {
	if enc == EncodingJSON {
		return NewJSONMessageEncoder()
	}
	return NewProtobufMessageEncoder()
}

// GetMessageDecoder ...
func GetMessageDecoder(enc Encoding) MessageDecoder {
	if enc == EncodingJSON {
		return NewJSONMessageDecoder()
	}
	return NewProtobufMessageDecoder()
}

var (
	jsonReplyEncoderPool     sync.Pool
	protobufReplyEncoderPool sync.Pool
)

// GetReplyEncoder ...
func GetReplyEncoder(enc Encoding) ReplyEncoder {
	if enc == EncodingJSON {
		e := jsonReplyEncoderPool.Get()
		if e == nil {
			return NewJSONReplyEncoder()
		}
		encoder := e.(ReplyEncoder)
		encoder.Reset()
		return encoder
	}
	e := protobufReplyEncoderPool.Get()
	if e == nil {
		return NewProtobufReplyEncoder()
	}
	encoder := e.(ReplyEncoder)
	encoder.Reset()
	return encoder
}

// PutReplyEncoder ...
func PutReplyEncoder(enc Encoding, e ReplyEncoder) {
	if enc == EncodingJSON {
		jsonReplyEncoderPool.Put(e)
		return
	}
	protobufReplyEncoderPool.Put(e)
}

// GetReplyDecoder ...
func GetReplyDecoder(enc Encoding, data []byte) ReplyDecoder {
	if enc == EncodingJSON {
		return NewJSONReplyDecoder(data)
	}
	return NewProtobufReplyDecoder(data)
}

// GetResultDecoder ...
func GetResultDecoder(enc Encoding) ResultDecoder {
	if enc == EncodingJSON {
		return NewJSONResultDecoder()
	}
	return NewProtobufResultDecoder()
}

// PutResultEncoder ...
func PutResultEncoder(enc Encoding, e ReplyEncoder) {
	return
}

// GetParamsEncoder ...
func GetParamsEncoder(enc Encoding) ParamsEncoder {
	if enc == EncodingJSON {
		return NewJSONParamsEncoder()
	}
	return NewProtobufParamsEncoder()
}

// PutParamsEncoder ...
func PutParamsEncoder(enc Encoding, e ParamsEncoder) {
	return
}

// GetCommandEncoder ...
func GetCommandEncoder(enc Encoding) CommandEncoder {
	if enc == EncodingJSON {
		return NewJSONCommandEncoder()
	}
	return NewProtobufCommandEncoder()
}
