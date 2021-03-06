package geerpc

import (
	"encoding/json"
	"errors"
	"geerpc/codec"
	"go/token"
	"io"
	"log"
	"net"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
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

type Server struct {
	ServiceMap sync.Map
}

type Request struct {
	h            *codec.Header
	argv, replyv reflect.Value

	service *service
	method  *methodType
}

type service struct {
	name    string
	typ     reflect.Type
	rcvr    reflect.Value
	methods map[string]*methodType
}

type methodType struct {
	method    reflect.Method
	argvType  reflect.Type
	replyType reflect.Type
	numCalls  uint64
}

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
	cc.Close()
}

func (s *Server) handleRequest(cc codec.Codec, req *Request, sending *sync.Mutex, wg *sync.WaitGroup) {
	defer wg.Done()

	err := req.service.call(req.method, req.argv, req.replyv)
	if err != nil {
		req.h.Error = err.Error()
	}
	s.sendResponse(cc, req.h, req.replyv.Interface(), sending)

}

func (s *Server) sendResponse(cc codec.Codec, h *codec.Header, body interface{}, sending *sync.Mutex) {
	sending.Lock()
	defer sending.Unlock()
	if err := cc.Write(h, body); err != nil {
		log.Println("rpc server: write response error:", err)
	}
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

	err = s.findServiceAndMethod(req)
	if err != nil {
		return req, err
	}

	m := req.method
	req.argv = m.newArgv()
	req.replyv = m.newReplyv()

	if req.argv.Kind() == reflect.Ptr {
		err = cc.ReadBody(req.argv.Interface())
	} else {
		err = cc.ReadBody(req.argv.Addr().Interface())
	}

	return req, err

}

func (s *Server) findServiceAndMethod(req *Request) error {
	dot := strings.LastIndex(req.h.ServiceMethod, ".")
	if dot < 0 {
		return errors.New("rpc: service/method request ill-formed: " + req.h.ServiceMethod)
	}

	serviceName := req.h.ServiceMethod[:dot]
	methodName := req.h.ServiceMethod[dot+1:]

	svci, ok := s.ServiceMap.Load(serviceName)
	if !ok {
		return errors.New("rpc: can't find service " + serviceName)
	}
	svc := svci.(*service)
	method, ok := svc.methods[methodName]
	if !ok {
		return errors.New("rpc: can't find method " + methodName)
	}

	req.service = svc
	req.method = method
	return nil
}

func (m *methodType) NumCalls() uint64 {
	return atomic.LoadUint64(&m.numCalls)
}

func (m *methodType) newArgv() reflect.Value {
	var argv reflect.Value

	if m.argvType.Kind() == reflect.Ptr {
		argv = reflect.New(m.argvType.Elem())
	} else {
		argv = reflect.New(m.argvType).Elem()
	}

	return argv
}

func (m *methodType) newReplyv() reflect.Value {

	replyv := reflect.New(m.replyType.Elem())

	switch m.replyType.Elem().Kind() {
	case reflect.Map:
		replyv.Elem().Set(reflect.MakeMap(m.replyType.Elem()))
	case reflect.Slice:
		replyv.Elem().Set(reflect.MakeSlice(m.replyType.Elem(), 0, 0))
	}
	return replyv
}

func newService(rcvr interface{}) *service {
	s := new(service)

	s.typ = reflect.TypeOf(rcvr)
	s.rcvr = reflect.ValueOf(rcvr)

	s.name = reflect.Indirect(s.rcvr).Type().Name()

	if !token.IsExported(s.name) {
		log.Fatalf("struct :%s is not a exported type\n", s.name)
	}

	s.registerMethods()

	return s
}

func (s *service) registerMethods() {
	s.methods = make(map[string]*methodType)

	for i := 0; i < s.typ.NumMethod(); i++ {
		method := s.typ.Method(i)
		mType := method.Type

		if !method.IsExported() {
			continue
		}
		if mType.NumIn() != 3 || mType.NumOut() != 1 {
			continue
		}
		argType, replyType := mType.In(1), mType.In(2)
		if !isExportedOrBuiltinType(argType) || !isExportedOrBuiltinType(replyType) {
			continue
		}
		if mType.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
			continue
		}

		s.methods[method.Name] = &methodType{
			method:    method,
			argvType:  argType,
			replyType: replyType,
		}

		log.Printf("service %s register method: %s", s.name, method.Name)

	}
}

func isExportedOrBuiltinType(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return token.IsExported(t.Name()) || t.PkgPath() == ""
}

func (s *service) call(m *methodType, argv, replyv reflect.Value) error {
	atomic.AddUint64(&m.numCalls, 1)
	f := m.method.Func

	returnValues := f.Call([]reflect.Value{s.rcvr, argv, replyv})
	if errInter := returnValues[0].Interface(); errInter != nil {
		return errInter.(error)
	}
	return nil
}

func (s *Server) Register(rcvr interface{}) error {

	service := newService(rcvr)

	if _, dup := s.ServiceMap.LoadOrStore(service.name, service); dup {
		return errors.New("rpc. Register: service already register: " + service.name)
	}

	return nil
}

func Register(rcvr interface{}) error {
	return DefaultServer.Register(rcvr)
}
