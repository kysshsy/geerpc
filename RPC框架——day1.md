# 服务端与消息编码

## 目标

- 定义消息格式
- 抽象出流式编码解码接口
- 完成简单的服务器端
- 测试发送接收消息功能

## 设计

### 1.定义消息格式

- Option

```asciiarmor
| Option{MagicNumber: xxx, CodecType: xxx} | Header{ServiceMethod ...} | Body interface{} |
| <------      固定 JSON 编码      ------>  | <-------   编码方式由 CodeType 决定   ------->|
```

Option表示一个rpc会话的开始，服务器端通过固定长度的Option来判断是否是正确RPC请求。首先是判断RPC请求开始的Magic Number，随后CodecType决定着后续请求的编码方式（以此支持协议交换，即支持多种rpc请求编码方式）。

Q: 为什么不用 定长的int 比如 int32/int64

```go
type Option struct {
	MagicNumber int				 //自定义的magic number
	CodecType   codec.Type //决定后续编码方式 just string
}
```

- Header

Header包含了RPC请求的请求方法，请求序号(seq number 用来分辨是哪个请求)，以及Error（？在客户端发送服务器端时或许可以去掉Error）

```go
type Header struct {
	ServiceMethod string
	Seq           uint64
	Error         string
}
```

- Body

Body则很随意，主要是使用一种编码方式encode请求的参数，在返回结果时，则包含rpc的结果。

***Q: gob为什么底层需要indirect ？***

### 2.抽象编解码类

Codec是一个描述编解码器的interface，其接口很简单。

```go
type Codec interface {
	io.Closer
	ReadHeader(*Header) error    //读取Header
	ReadBody(interface{}) error  //读取Body
	Write(*Header, interface{}) error //写入 响应
}
```

经过对golang官方库的学习，了解到io.Closer也是interface，表示接口应该实现`Close() error`。





### 3. 服务器端

观察golang的官方服务器实现（http服务器、rpc服务器）会发现服务器通常采用以下模式。

1. 监听listener

```go
for {
		conn, err := lis.Accept()
		if err != nil {
			log.Printf("rpc server: listen error\n")
		}
		go s.ServeConn(conn)
	}
```

采用一个死循环监听一个listener，随后使用goroutine处理请求

2. 监听会话

若收到请求，经过处理后抽离出context进行处理。由于rpc请求相互独立，这里再次使用goroutine处理每一个rpc请求（做好同步与发送的互斥）。

## 实现

### 1.CodeC示例

使用官方库进行编解码，细节不多。值的注意的是使用bufio缓冲，但是写入时需要注意刷新缓冲区。

```go
// 值的注意的是可以使用buf进行写入 缓冲，提高效率
func NewGobCodec(conn io.ReadWriteCloser) Codec {
	buf := bufio.NewWriter(conn)
	return &GobCodec{
		conn:    conn,
		buf:     buf,
		decoder: gob.NewDecoder(conn),
		encoder: gob.NewEncoder(buf),
	}
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
```

### 2.Server实现

server实现需要Accept流程的划分和注意goroutine的使用。

```go
func (s *Server) Accept(lis net.Listener) {
	for {
		conn, err := lis.Accept()
		if err != nil {
			log.Printf("rpc server: listen error\n")
		}
		go s.ServeConn(conn) //本体在无限循环 使用goroutine进行处理conn
	}
}

func (s *Server) ServeConn(conn io.ReadWriteCloser) {
  defer func() {
    _ = conn.Close() // 极客🐰实现 可能有问题
  }
  sending sync.Mutex
  wg sync.Waitgroup
  for {
    包装成codec
  	// 接收请求后 读取 header 和body
    wg.Add(1) 
    go s.serveRequest(codec, header, body, sending, wg) //由于 rpc请求可以并行请求/处理 需要做好互斥及同步
  }
  // wait wg
  wg.Wait()
}

```

***Q: 编解码器为什么需要实现 Close方法？直接Close conn不行吗？ Close conn多次是否有影响***

[close多次](https://stackoverflow.com/questions/64407661/is-gos-net-close-idempotent),极客🐰的实现可能有多次关闭的问题⚠️。

应该将CodeC的close改造成幂等接口，能够多次调用。并且在conn的层面 不要关闭

## 知识点

1. 熟悉io interface 流式接口

> Go comes with many APIs that support streaming IO from resources like in-memory structures, files, network connections, to name a few. 

go将文件，网络等包装成流式接口，gob、json等编解码器可以通过这些流式接口再包装一层，使得输入和输出的数据经过gob、json编解码，同时它还是一个流式的接口。这被称为chaining。

***Q：了解gob json 编解码的实现 如何做到区分data的边界***

2. 再次熟悉一下网络接口 listen accept等
3. 了解gob json，特别是gob即go binary的作用以及实现。

--------------

拓展了解：gob json编解码实现