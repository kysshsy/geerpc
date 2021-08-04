package main

import (
	"encoding/json"
	"fmt"
	"geerpc"
	"geerpc/codec"
	"log"
	"net"
	"time"
)

func startServer(addr chan string) {
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		log.Printf("main: startServer error\n")
		panic("startServer")
	}
	log.Println("start rpc server on", lis.Addr())

	server := geerpc.NewServer()
	addr <- lis.Addr().String()

	server.Accept(lis)

}

func main() {
	addr := make(chan string)
	go startServer(addr)

	time.Sleep(time.Second)

	conn, err := net.Dial("tcp", <-addr)

	if err != nil {
		panic("connect eroor")
	}

	time.Sleep(time.Second)

	_ = json.NewEncoder(conn).Encode(geerpc.DefaultOption)

	cc := codec.NewGobCodec(conn)

	for i := 0; i < 5; i++ {
		h := &codec.Header{
			ServiceMethod: "Foo.Sum",
			Seq:           uint64(i),
		}
		_ = cc.Write(h, fmt.Sprintf("geerpc req %d", h.Seq))
		_ = cc.ReadHeader(h)
		var reply string
		_ = cc.ReadBody(&reply)
		log.Println("reply:", reply)

	}
}
