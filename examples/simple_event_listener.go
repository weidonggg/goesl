package main

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/weidonggg/goesl"
)

func main() {
	conn := goesl.NewConnection("localhost:8021", "ClueCon")

	conn.SetEventHandler(func(event *goesl.Event) {
		eventName := event.Header["Event-Name"]
		log.Printf("事件: %s", eventName)

		if eventName == "CHANNEL_CREATE" {
			log.Printf("通道创建 - UUID: %s, Caller: %s -> Callee: %s",
				event.Header["Unique-ID"],
				event.Header["Caller-Caller-ID-Number"],
				event.Header["Caller-Destination-Number"])
		}

		if eventName == "CUSTOM" {
			subclass := event.Header["Event-Subclass"]
			if decoded, err := url.QueryUnescape(subclass); err == nil {
				subclass = decoded
			}
			log.Printf("自定义事件 - 子类: %s", subclass)
		}
	})

	conn.SetOnDisconnect(func() {
		log.Println("连接已断开")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := conn.Connect(ctx); err != nil {
		log.Fatalf("连接失败: %v", err)
	}
	log.Println("连接成功！")

	if _, err := conn.SubscribeEventsPlain("ALL"); err != nil {
		log.Fatalf("订阅事件失败: %v", err)
	}
	log.Println("订阅事件成功！")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("正在关闭连接...")
		conn.Close()
		cancel()
	}()

	for {
		select {
		case err := <-conn.ErrorChannel():
			log.Printf("错误: %v", err)
			if err == goesl.ErrConnectionClosed {
				return
			}
		case <-conn.EventChannel():
		case <-time.After(1 * time.Second):
			if conn.IsConnected() {
				fmt.Print(".")
			}
		}

		if !conn.IsConnected() {
			log.Println("连接已断开")
			return
		}
	}
}
