package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/weidonggg/goesl"
)

type CustomLogger struct{}

func (l *CustomLogger) Debug(format string, args ...interface{}) {
	log.Printf("[DEBUG] "+format, args...)
}

func (l *CustomLogger) Info(format string, args ...interface{}) {
	log.Printf("[INFO] "+format, args...)
}

func (l *CustomLogger) Warn(format string, args ...interface{}) {
	log.Printf("[WARN] "+format, args...)
}

func (l *CustomLogger) Error(format string, args ...interface{}) {
	log.Printf("[ERROR] "+format, args...)
}

func main() {
	log.SetOutput(os.Stdout)

	address := "localhost:8021"
	password := "ClueCon"

	fmt.Println("=== FreeSWITCH ESL 带日志示例 ===")
	fmt.Println()

	conn := goesl.NewConnection(address, password)

	conn.SetLogger(&CustomLogger{})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fmt.Println("正在连接...")
	if err := conn.Connect(ctx); err != nil {
		log.Fatalf("连接失败: %v", err)
	}
	fmt.Println("连接成功！")
	fmt.Println()

	fmt.Println("获取系统版本...")
	version, err := conn.SendAPI("version")
	if err != nil {
		log.Printf("获取版本失败: %v", err)
	} else {
		fmt.Printf("版本: %s\n", version.Body)
	}
	fmt.Println()

	fmt.Println("订阅事件...")
	_, err = conn.SubscribeEventsPlain("ALL")
	if err != nil {
		log.Fatalf("订阅失败: %v", err)
	}
	fmt.Println("订阅成功！")
	fmt.Println()

	fmt.Println("监听事件 (5 秒)...")
	select {
	case err := <-conn.ErrorChannel():
		log.Printf("错误: %v", err)
	case event := <-conn.EventChannel():
		if event != nil {
			fmt.Printf("收到事件: %s\n", event.Header["Event-Name"])
		}
	case <-time.After(5 * time.Second):
		fmt.Println("测试完成")
	}

	fmt.Println()
	fmt.Println("关闭连接...")
	conn.Close()
	fmt.Println("已关闭")
}
