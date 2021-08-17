package main

import (
	"fmt"
	"geerpc"
	"log"
	"net"
	"sync"
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

	client, err := geerpc.Dail("tcp", <-addr)
	if err != nil {
		log.Panic("rpc client init error")
	}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {

		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := fmt.Sprintf("geerpc req %d", i)
			var reply string
			if err := client.Call("Foo.Sum", args, &reply); err != nil {
				log.Fatal("call Foo.Sum error:", err)
			}
			log.Println("reply:", reply)
		}(i)

	}
	wg.Wait()
}
