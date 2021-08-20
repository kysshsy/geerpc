# rpc客户端

## 目标

实现一个并发的rpc客户端

- [x] 与服务器建立连接，确定编码格式
- [x] 记录rpc调用
- [x] 提供阻塞接口和非阻塞接口
- [x] 并发发送rpc请求

## 设计

### 1.记录rpc调用

client使用一个map记录rpc的seq对应的调用`map[uint64]*Call`,其中Call用来记录单次的rpc调用。

自然地，我们可以想到Call，应该有以下几个元素

- ServiceMethod  方法名
- Seq                      rpc序号
- Args                     rpc参数
- Reply                   rpc结果

除此之外，Call还包含

- Err                      错误
- Done                  标识调用结束

```go
type Call struct {
	ServiceMethod string
	Seq           uint64
	Err           error
	Args          interface{}
	Replys        interface{}
	Done          chan *Call  //重要 标识调用已完成
}

Done通常是一个 buffered chan 
则接收者 会阻塞在 <-Done, 而发送者不会阻塞在Done<-

rpc官方库中 Done为 buffered 10,Done除了标识当前Call完成外 还可以在rpc请求时传入同一个Done
限制client并发rpc的数量
```

### 2. 并发发送

并发发送其实很简单，主要是使用mutex保护client记录调用的map，以及发送时使用mutex保护io接口。

client使用一个map保存已经发送的rpc，key为rpc的序号，value则是之前提到的Call。为了并发使用客户端，则需要使用一个mutex来保护client的数据，并且另外使用一个mutex保护io字节流。[可以看stackOverflow讨论](https://stackoverflow.com/questions/63283110/why-does-net-rpc-clients-go-method-require-a-buffered-channel)

### 3.接收

client除了发送请求还需要异步地接收返回，所以使用一个goroutine在等待服务器的返回。主要的编码细节是注意区分错误

1. 发生通信错误则关闭client，清空所有Call
2. 发生流程上的错误(比如接收到的Seq对应Call不存在)，则需要使用nil读取一下字节流中的数据。

```go
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
			err = c.cc.ReadBody(nil)
		case header.Error != "":
			call.Err = fmt.Errorf(header.Error)
			err = c.cc.ReadBody(nil)
			call.done()
		default:
			err = c.cc.ReadBody(call.Replys)
			if err != nil {
				call.Err = errors.New("read body" + err.Error())
			}
			call.done()
		}
	}
	c.terminateCall(err)
}
```



## 闪光点

### 1.使用一个buffered channel使得接收异步

发送goroutine等待channel，客户端接收部分则通过这个buffered chan ` *Call`,异步地返回server传回的response。发送goroutine则可以通过` *Call`访问到Call（保存着调用的请求和返回）。

### 2. for range map过程中删除map元素 safe～

```go
for key := range m {
    if key.expired() {
        delete(m, key)
    }
}
```

这样是安全的，但是如果是增加的话，for range可能不会遍历到

https://stackoverflow.com/questions/23229975/is-it-safe-to-remove-selected-keys-from-map-within-a-range-loop

### 3. Named return value

Named return value在golang中是指

```go
package main

import "fmt"

func split(sum int) (x, y int) {
	x = sum * 4 / 9
	y = sum - x
	return
}

func main() {
	fmt.Println(split(17))
}

```

在函数签名中指定返回值的名字，这样返回值会被赋值到这个变量，或者直接return 返回该变量。

```go
func Dail(network, addr string, option ...*Option) (client *Client, err error) {
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
```

return func后函数的值被赋值到 client和err中，defer函数中可以使用这个返回值进行操作。



------------------



