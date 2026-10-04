# Logs

`github.com/fino-io/finokit/logs` 复用 Core 的日志实现，负责加载 finokit 配置、传递上下文字段，并将 OpenTelemetry 的 `trace_id`、`span_id` 加入日志。

日志输出、级别控制和文件资源管理由 `github.com/fino-io/core/go/logs` 提供。`Config`、`FileConfig`、`Level`、`Logger`、`Field`、`Entry` 直接使用 Core 类型；finokit 的 `Service` 在 Core 服务上保留 trace 行为，`WithContext`、`WithLogger` 返回派生服务。

## 使用

```go
logger := logs.NewLoggerWith(&logs.Config{
    Level:  "info",
    Encode: "console",
    Output: "console",
})
logs.SetLogger(logger)
logs.Infow("service started", "name", "api")

ctx = logs.WithFields(ctx, logs.Field{Key: "request_id", Value: "request-1"})
logs.Ctx(ctx).Infow("request handled", "result", "ok")
```

上下文存在有效 span 时，真实 trace 信息覆盖调用方提供的同名字段；字段合并不会修改调用方的切片。

独立服务可按上下文和 logger 派生，原服务保持不变：

```go
service := logs.NewService(logger).WithContext(ctx)
worker := service.WithLogger(workerLogger)
worker.Infow("worker ready", "id", 7)
```

## 配置

初始化 finokit 配置后，调用 `logs.NewConfig()` 加载 `logs` 节点并替换默认 logger。读取失败时保留原 logger。

```yaml
logs:
  level: info
  encode: console
  output: console
  file:
    path: ./logs/app.log
    encode: json
    maxSize: 100
    maxBackups: 10
    maxAge: 30
```

`output: file` 使用文件配置，控制台输出使用顶层 `encode`。完整配置类型和默认值由 Core 提供。

## 动态级别和资源生命周期

将 logger 的 handler 挂载到应用已有的 HTTP 服务：

```go
mux.Handle("/log/level", logger.LevelHandler())
```

`GET /log/level` 获取级别，`PUT /log/level` 设置级别；请求体支持 JSON `{"level":"debug"}` 或表单 `level=debug`。派生 logger 共享级别。

关闭文件 logger 前调用 `Sync()`，停止所有共享该文件资源的使用者后调用 `Close()`。派生 logger 共享资源，重复关闭只释放一次；替换默认 logger 时，由应用管理旧 logger 的关闭时机。

自定义 `Logger` 需要实现 `SetLevel`、`GetLevel`、`With`、`Log`、`LevelHandler`、`Sync`、`Close`。finokit 的 trace 包装会透传级别、handler 和生命周期方法。
