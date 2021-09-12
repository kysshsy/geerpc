package codec

import "io"

type Header struct {
	ServiceMethod string
	Seq           uint64
	Error         string
}

type Codec interface {
	io.Closer
	ReadHeader(*Header) error
	ReadBody(interface{}) error
	Write(*Header, interface{}) error
}

type Type string

const (
	GobType  = "application/gob"
	JsonType = "application/json"
)

type NewCodecFunc func(closer io.ReadWriteCloser) Codec

var CodecFuncMap map[Type]NewCodecFunc

func init() {
	CodecFuncMap = make(map[Type]NewCodecFunc)

	CodecFuncMap[GobType] = NewGobCodec
	CodecFuncMap[JsonType] = NewJsonCodec
}
