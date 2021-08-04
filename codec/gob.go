package codec

import (
	"bufio"
	"encoding/gob"
	"io"
	"log"
)

var _ Codec = (*GobCodec)(nil)

type GobCodec struct {
	conn    io.ReadWriteCloser
	buf     *bufio.Writer
	decoder *gob.Decoder
	encoder *gob.Encoder
}

func NewGobCodec(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn)
	return &GobCodec{
		conn:    conn,
		buf:     buf,
		decoder: gob.NewDecoder(conn),
		encoder: gob.NewEncoder(buf),
	}

}

func (gob *GobCodec) Close() error {
	return gob.conn.Close()
}

func (gob *GobCodec) ReadHeader(header *Header) error {
	return gob.decoder.Decode(header)
}

func (gob *GobCodec) ReadBody(body interface{}) error {
	return gob.decoder.Decode(body)
}
func (gob *GobCodec) Write(h *Header, data interface{}) (err error) {
	defer func() {
		_ = gob.buf.Flush()
		if err != nil {
			err = gob.conn.Close() // 改变了返回值
		}
	}()

	if err = gob.encoder.Encode(h); err != nil {
		log.Println("rpc codec: gob Write Header fail")
		return
	}

	if err = gob.encoder.Encode(data); err != nil {
		log.Println("rpc codec: gob Write Body fail")
		return
	}
	return
}
