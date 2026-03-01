# FreeSWITCH ESL 示例

这个目录包含使用 goesl 库连接 FreeSWITCH ESL (Event Socket Library) 的示例程序。

## 示例程序

### 1. 简单事件监听器 (`simple_event_listener.go`)

最简单的 FreeSWITCH 事件监听示例，演示基本的连接和事件接收功能。

**功能特点：**
- 连接到 FreeSWITCH ESL 服务器
- 订阅所有事件
- 处理基本通道创建事件
- 优雅的退出处理

**运行方法：**
```bash
go run examples/simple_event_listener.go
```

**代码示例：**
```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"

    "github.com/weidonggg/goesl"
)

func main() {
    conn := goesl.NewConnection("host:port", "password")

    conn.SetEventHandler(func(event *goesl.Event) {
        eventName := event.Header["Event-Name"]
        log.Printf("事件: %s", eventName)
    })

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    if err := conn.Connect(ctx); err != nil {
        log.Fatalf("连接失败: %v", err)
    }

    if _, err := conn.SubscribeEventsPlain("ALL"); err != nil {
        log.Fatalf("订阅事件失败: %v", err)
    }

    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan

    conn.Close()
}
```

### 2. 自定义日志示例 (`with_logger.go`)

演示如何使用自定义日志记录器来控制 goesl 库的输出。

**功能特点：**
- 自定义日志记录器实现
- 控制不同级别的日志输出
- 演示库的调试信息输出

**运行方法：**
```bash
go run examples/with_logger.go
```

**代码示例：**
```go
package main

import (
    "context"
    "log"
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
    conn := goesl.NewConnection("host:port", "password")
    conn.SetLogger(&CustomLogger{})

    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()

    if err := conn.Connect(ctx); err != nil {
        log.Fatalf("连接失败: %v", err)
    }

    conn.Close()
}
```

## 核心 API

### 连接管理

```go
conn := goesl.NewConnection("host:port", "password")
err := conn.Connect(ctx)
conn.Close()
```

### 事件订阅

```go
conn.SubscribeEventsPlain("ALL")
conn.SubscribeEventsJSON("CHANNEL_CREATE", "CHANNEL_ANSWER")
```

### 发送命令

```go
conn.SendCommand("status")
conn.SendAPI("show channels")
```

### 事件处理

```go
conn.SetEventHandler(func(event *goesl.Event) {
    eventName := event.Header["Event-Name"]
    log.Printf("收到事件: %s", eventName)
})
```

### URL 解码

某些 FreeSWITCH 事件字段可能包含 URL 编码的值（如 `sofia%3A%3Aregister`），使用方需要自行解码：

```go
import "net/url"

conn.SetEventHandler(func(event *goesl.Event) {
    eventName := event.Header["Event-Name"]
    
    if eventName == "CUSTOM" {
        subclass := event.Header["Event-Subclass"]
        if decoded, err := url.QueryUnescape(subclass); err == nil {
            subclass = decoded
        }
        log.Printf("自定义事件 - 子类: %s", subclass)
    }
})
```

常见的 URL 编码字符：
- `:` → `%3A`
- `/` → `%2F`
- `?` → `%3F`
- `&` → `%26`
- 空格 → `%20`

## 错误处理

```go
conn, err := goesl.NewConnection("host:port", "password")
if err != nil {
    log.Fatalf("创建连接失败: %v", err)
}

if err := conn.Connect(ctx); err != nil {
    log.Fatalf("连接失败: %v", err)
}

response, err := conn.SendCommand("status")
if err != nil {
    log.Fatalf("发送命令失败: %v", err)
}

if !response.OK {
    log.Printf("命令执行失败: %s", response.Reply)
}
```

## 事件数据结构

```go
type Event struct {
    Header map[string]string  // 事件头部信息
    Body   string           // 事件体内容
    Raw    string           // 完整的原始消息
}
```

## 常见 FreeSWITCH 事件

- `CHANNEL_CREATE`: 新通道创建
- `CHANNEL_ANSWER`: 通道应答
- `CHANNEL_HANGUP`: 通道挂断
- `DTMF`: DTMF 按键
- `CUSTOM`: 自定义事件

## 更多信息

- [FreeSWITCH ESL 文档](https://freeswitch.org/confluence/display/FREESWITCH/Event+Socket+Library)
- [goesl 库文档](https://github.com/weidonggg/goesl)
