package geerpc

import (
	"encoding/json"
	"fmt"
	"geerpc/codec"
	"io"
	"log"
	"net"
	"reflect"
	"sync"
)

const MagicNumber = 0x3bef5c

type Option struct {
	MagicNumber int
	CodecType   codec.Type
}

var DefaultOption = &Option{
	MagicNumber: MagicNumber,
	CodecType:   codec.GobType,
}

type Server struct{}

func NewServer() *Server {
	return &Server{}
}

var DefaultServer = NewServer()

func (s *Server) Accept(lis net.Listener) {
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Printf("rpc server: listen error\n")
		}
		go s.ServeConn(conn)
	}
}

func Accept(lis net.Listener) { DefaultServer.Accept(lis) }

func (s *Server) ServeConn(conn io.ReadWriteCloser) {
	defer func() {
		_ = conn.Close()
	}()

	dec := json.NewDecoder(conn)

	var option Option

	if err := dec.Decode(&option); err != nil {
		log.Println("rpc server: options error: ", err)
		return
	}

	if option.MagicNumber != MagicNumber {
		log.Printf("rpc server: invalid magic number: %x\n", option.MagicNumber)
		return
	}
	f := codec.CodecFuncMap[option.CodecType]
	if f == nil {
		log.Printf("rpc server: no valid codecFunc: %s", option.CodecType)
		return
	}
	s.serveConn(f(conn))
}

var invalidResquest = struct{}{}

func (s *Server) serveConn(cc codec.Codec) {
	var (
		send sync.Mutex
		wg   sync.WaitGroup
	)

	for {
		req, err := s.readRequest(cc)

		if err != nil {
			if req == nil {
				break
			}
			// 发送回应 错误
			req.h.Error = err.Error()
			s.sendResponse(cc, req.h, invalidResquest, &send)
			continue
		}

		wg.Add(1)

		go s.handleRequest(cc, req, &send, &wg)
	}

	wg.Wait()

}

func (s *Server) handleRequest(cc codec.Codec, req *Request, sending *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Println(req.h, req.argv.Elem())
	req.repv = reflect.ValueOf(fmt.Sprintf("geerpc resp %d", req.h.Seq))
	s.sendResponse(cc, req.h, req.repv.Interface(), sending)

}

func (s *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Write(h, body); err != nil {
		log.Println("rpc server: write response error:", err)
	}
}

type Request struct {
	h          *codec.Header
	argv, repv reflect.Value
}

func (s *Server) readRequestHeader(cc codec.Codec) (*codec.Header, error) {
	h := &codec.Header{}

	if err := cc.ReadHeader(h); err != nil {
		return nil, err
	}
	return h, nil
}

func (s *Server) readRequest(cc codec.Codec) (*Request, error) {

	h, err := s.readRequestHeader(cc)
	if err != nil {
		return nil, err
	}

	req := &Request{h: h}

	req.argv = reflect.New(reflect.TypeOf(""))
	if err := cc.ReadBody(req.argv.Interface()); err != nil {
		log.Printf("rpc server: read argv error: %s\n", err.Error())
	}

	return req, err
}
