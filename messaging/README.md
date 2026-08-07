# Messaging

`messaging` 提供一层很薄的消息队列抽象，统一发布、订阅、消息上下文和确认语义。当前目录定义了通用模型和 `Client`，具体实现放在子包里，例如 [`nats`](./nats)。

## 适用场景

- 业务代码只依赖统一接口，不直接耦合到底层消息队列 SDK。
- 发布和订阅都通过同一套 `Message` / `Subscription` 模型表达。
- 订阅处理函数可以从 `context.Context` 中拿到 topic、message id 和 attributes。

## 核心类型

### `Message`

发布消息时使用的基础结构：

```go
type Message struct {
    Id         string
    TraceId    string
    SpanId     string
    Attributes map[string]any
    Data       string
}
```

其中 `Data` 是消息体字符串，额外元数据可以放在 `Attributes`。

### `Subscription`

订阅配置的公共模型：

```go
type Subscription struct {
    Name              string
    Topic             string
    Group             string
    Pull              bool
    AckWait           time.Duration
    PullMaxWaiting    int
    PendingMsgLimit   int
    PendingBytesLimit int
    Endpoint          Endpoint
}
```

- `Topic` 不能为空，`Client.Subscribe` 和各具体实现都会复用 `Validate()` 做校验。
- `Group` 非空时，底层实现可以按队列订阅方式消费。

### `SubMessage`

消费时传给 handler 的消息对象，继承 `Message` 字段，并带确认能力：

- `Ack()`：确认消息。
- `Nak()`：拒绝并允许重投。
- `Term()`：终止消息。
- `InProgress()`：告知底层消息仍在处理中。
- `Done()`：是否已经执行过一次终结动作。

`Ack` / `Nak` / `Term` 只会生效一次，避免重复确认。

## 快速开始

### 1. 通过配置初始化

推荐直接使用根包配置：

```go
package main

import (
    "log"

    "github.com/fino-io/finokit/messaging"
    "github.com/fino-io/finokit/messaging/nats"
)

func main() {
    nats.Register()

    client, err := messaging.New()
    if err != nil {
        log.Fatal(err)
    }
    defer client.Shutdown()
}
```

默认会读取 `messaging` 配置项，对应 [messaging/config/messaging.yaml](./config/messaging.yaml)：

```yaml
messaging:
  driver: nats
  nats:
    url: nats://127.0.0.1:4222
    jetStream: true
    streams:
      - name: demo
        subjects:
          - demo.>
  subscriptions:
    - name: agent-consumer
      topic: demo.start-task
      group: start-task
      pull: true
      ackWait: 5m
      endpoint:
        service: Agent
        method: start_task
```

#### NATS 配置字段

- `driver`：消息队列驱动，当前 NATS 实现使用 `nats`。
- `nats.url`：NATS Server 的连接地址。
- `nats.jetStream`：是否启用 JetStream。启用后支持消息持久化、消费确认和重投；关闭时使用 Core NATS，`streams` 不生效。
- `nats.streams`：应用启动时需要确保存在的 JetStream Stream 列表。Stream 不存在时自动创建，已经存在时复用并同步 `subjects`。
- `streams[].name`：JetStream Stream 的名称，在同一个 NATS account 中必须唯一。它是服务端资源名，不是发布消息时使用的 topic。
- `streams[].subjects`：该 Stream 接收并保存的 subject 列表。一个 Stream 可以匹配多个 subject。

NATS subject 使用 `.` 分段，支持以下通配符：

- `*` 只匹配一个分段，例如 `demo.*` 匹配 `demo.start-task`，但不匹配 `demo.task.started`。
- `>` 匹配后续一个或多个分段，并且只能放在末尾。例如 `demo.>` 匹配 `demo.start-task` 和 `demo.task.started`，但不匹配 `demo` 或 `user.start-task`。

上面的配置会把发布到 `demo.start-task` 等 `demo.>` subject 的消息保存到名为 `demo` 的 Stream，再由 `subscriptions` 中的 consumer 消费。

### 2. 手动初始化底层队列

如果你不想依赖外部配置，也可以直接构造具体队列：

```go
package main

import (
    "log"

    "github.com/fino-io/finokit/messaging"
    "github.com/fino-io/finokit/messaging/nats"
)

func main() {
    queue, err := nats.NewWithConfig(&messaging.NatsConfig{
        URL:       "nats://127.0.0.1:4222",
        JetStream: true,
        Streams: []*messaging.NatsStream{{
            Name:     "demo",
            Subjects: []string{"demo.>"},
        }},
    })
    if err != nil {
        log.Fatal(err)
    }
    defer queue.Shutdown()

    client, err := messaging.NewWithQueue(queue)
    if err != nil {
        log.Fatal(err)
    }
    defer client.Shutdown()
}
```

### 3. 发布消息

```go
err := client.Publish(ctx, "demo.user.created", &messaging.Message{
    Id:      "msg-1",
    TraceId: "trace-1",
    Attributes: map[string]any{
        "source": "api",
    },
    Data: `{"userId":"u-1"}`,
})
```

发布前会校验 topic 非空。

### 4. 订阅消息

```go
err := client.Subscribe(&messaging.Subscription{
    Name:  "user-created-worker",
    Topic: "demo.user.created",
    Group: "user-workers",
}, func(ctx context.Context, sub *messaging.Subscription, msg *messaging.SubMessage) error {
    topic := messaging.GetContextTopic(ctx)
    messageID := messaging.GetContextMessageId(ctx)
    attrs := messaging.GetContextMessageAttributes(ctx)

    _ = topic
    _ = messageID
    _ = attrs

    // 处理 msg.Data
    return nil
})
```

默认行为：

- JetStream handler 返回 `nil` 且消息还没结束时，框架自动 `Ack()`。
- JetStream handler 返回错误且消息还没结束时，框架自动 `Nak()`。
- 如果你在 handler 内主动调用了 `Ack()` / `Nak()` / `Term()`，后续不会重复执行。
- Core NATS 没有消息确认协议，不执行上述确认操作。

## 显式终结示例

不可重试的消息可以显式 `Term()`；其他成功和失败路径交给统一确认策略：

```go
err := client.Subscribe(&messaging.Subscription{
    Name:  "task-worker",
    Topic: "demo.task",
}, func(ctx context.Context, sub *messaging.Subscription, msg *messaging.SubMessage) error {
    if msg.Data == "" {
        msg.Term()
        return nil
    }

    return handle(msg.Data)
})
```

## 上下文辅助函数

订阅回调里可以使用这些 helper：

- `GetContextTopic(ctx)`：获取当前 topic。
- `GetContextMessage(ctx)`：获取完整 `*SubMessage`。
- `GetContextMessageId(ctx)`：获取消息 ID。
- `GetContextMessageAttributes(ctx)`：获取消息 attributes。

如果你需要把消息信息继续透传到下游，也可以使用：

- `WithTopic(ctx, topic)`
- `WithMessage(ctx, msg)`
- `WithMessageID(ctx, id)`
- `WithMessageAttributes(ctx, attrs)`

## 错误约定

常见输入校验错误：

- `ErrNilClient`
- `ErrNilQueue`
- `ErrNilSubscription`
- `ErrNilHandler`
- `ErrEmptyTopic`

这些错误用于尽早暴露调用方传参问题。

## 配置驱动订阅

`New` 和 `NewWithConfig` 创建的 client 会保留配置中的订阅，可以由服务启动代码统一路由：

```go
for _, subscription := range client.Subscriptions() {
    if subscription.Endpoint.Service != "Agent" {
        continue
    }
    if err := client.Subscribe(subscription, handler); err != nil {
        return err
    }
}
```

NATS 直接读取 `Subscription` 上的 pull、ack wait 和 pending limit，无需再转换成另一层 consumer 类型。
