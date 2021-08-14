package geerpc

import (
	"encoding/json"
	"errors"
	"geerpc/codec"
	"io"
	"net"
	"sync"
)

func Dail(network, addr string, option ...*Option) (client *Client, err error) {
	opt, err := parseOption(option)
	if err != nil {
		return nil, err
	}

	conn, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}

	defer func() {
		if client == nil { // 这个可厉害了
			_ = conn.Close()
		}
	}()

	return NewClient(conn, opt)
}

func NewClient(conn io.ReadWriteCloser, option *Option) (*Client, error) {
	codecFunc, ok := codec.CodecFuncMap[option.CodecType]
	if !ok {
		return nil, errors.New("get codecfunc error")
	}

	if err := json.NewEncoder(conn).Encode(option); err != nil {
		return nil, err
	}
	return newClientWithCodec(codecFunc(conn), option), nil
}

func newClientWithCodec(cc codec.Codec, option *Option) *Client {
	client := &Client{
		seq:     1,
		cc:      cc,
		opt:     option,
		pending: make(map[uint64]*Call),
	}

	go client.receive()

	return client
}

func parseOption(option []*Option) (*Option, error) {
	if len(option) == 0 || option[0] == nil {
		return DefaultOption, nil
	}
	if len(option) != 1 {
		return nil, errors.New("too many options")
	}

	op := option[0]
	op.MagicNumber = DefaultOption.MagicNumber
	if op.CodecType == "" {
		op.CodecType = DefaultOption.CodecType
	}
	return op, nil

}

type Call struct {
	ServiceMethod string
	Seq           uint64
	Err           string
	Args          interface{}
	Replys        interface{}
	Done          chan *Call
}

func (c *Call) done() {
	c.Done <- c
}

type Client struct {
	cc       codec.Codec
	opt      *Option
	sending  sync.Mutex
	header   codec.Header
	mu       sync.Mutex
	seq      uint64
	pending  map[uint64]*Call
	closing  bool
	shutdown bool
}

var _ io.Closer = (*Client)(nil)

var ErrShutdown = errors.New("connection is shut down")

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closing {
		return ErrShutdown
	}
	c.closing = true
	return c.cc.Close()
}

func (c *Client) IsAvailable() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return !(c.closing || c.shutdown)
}

func (c *Client) registerCall(call *Call) (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closing || c.shutdown {
		return 0, ErrShutdown
	}

	call.Seq = c.seq
	c.pending[c.seq] = call
	c.seq++
	return call.Seq, nil
}

func (c *Client) removeCall(seq uint64) *Call {
	c.mu.Lock()
	defer c.mu.Unlock()

	call, ok := c.pending[seq]
	if !ok {
		return nil
	}
	delete(c.pending, seq)
	return call

}

func (c *Client) terminateCall(err error) {
	c.sending.Lock()
	defer c.sending.Unlock()
	c.mu.Lock()
	defer c.mu.Unlock()

	c.shutdown = true

	for k, v := range c.pending {
		v.Err = err.Error()
		//Q：map 删除会不会破坏range的顺序 A: it's safe
		delete(c.pending, k)
		v.done()
	}
}

func (c *Client) receive() {
	var err error
	for err == nil {
		var header codec.Header
		if err = c.cc.ReadHeader(&header); err != nil {
			break
		}
		call := c.removeCall(header.Seq)

		switch {
		case call == nil:
			err = c.
		}
	}
	c.terminateCall(err)
}
