package codec

import (
	"bufio"
	"encoding/json"
	"io"
	"log"
)

var _ Codec = (*JsonCodec)(nil)

type JsonCodec struct {
	conn io.ReadWriteCloser
	buf  *bufio.Writer
	dec  *json.Decoder
	enc  *json.Encoder
}

func NewJsonCodec(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn)

	return &JsonCodec{
		conn: conn,
		buf:  buf,
		dec:  json.NewDecoder(conn),
		enc:  json.NewEncoder(buf),
	}
}

func (cc *JsonCodec) Close() error {
	return cc.conn.Close()
}

func (cc *JsonCodec) ReadHeader(h *Header) error {
	return cc.dec.Decode(h)
}

func (cc *JsonCodec) ReadBody(b interface{}) error {
	return cc.dec.Decode(b)
}

func (cc *JsonCodec) Write(h *Header, b interface{}) (err error) {
	defer func() {
		_ = cc.buf.Flush()
		if err != nil {
			err = cc.conn.Close()
		}
	}()

	if err := cc.enc.Encode(h); err != nil {
		log.Println("rpc codec: gob Write Header fail")
		return
	}

	if err := cc.enc.Encode(b); err != nil {
		log.Println("rpc codec: gob Write Body fail")
		return
	}
	return
}
