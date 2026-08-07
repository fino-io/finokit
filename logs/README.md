# Logs

`logs` 是对 `github.com/fino-io/core/go/logs` 的兼容封装。

- 新代码优先直接使用 `logs`
- `logs` 保留旧包路径和调用方式
- 底层实现、配置结构和日志行为都直接来自 `logs`
- 当前兼容层主要集中在 `logs/logs.go`

## 与 `logs` 的关系

`logs` 只保留兼容 API，不再维护独立实现。

对应关系：

- `logs.Config` / `FileConfig` / `Level` / `Logger` / `Service` 都是 `logs` 的别名
- `logs.NewLoggerWith`、`SetLogger`、`SetLogLevel` 最终都调用 `logs`
- 包级 `Info`、`Infow`、`Errorf` 等函数通过 `logs.Service` 保持旧调用方式和 caller 行为

## 动态日志级别

当同时配置 `levelPattern` 和 `levelPort` 时，底层 logger 会启动一个 HTTP 服务，暴露 `zap.AtomicLevel.ServeHTTP`：

- `GET <levelPattern>`：获取当前日志级别
- `PUT <levelPattern>`：动态修改日志级别

支持两种 `PUT` 请求体：

- `application/json`，例如 `{"level":"debug"}`
- `application/x-www-form-urlencoded`，例如 `level=debug`

## 主要接口

```go
logger := logs.NewLoggerWith(&logs.Config{
Level:  "info",
Encode: "console",
Output: "console",
})

logs.SetLogger(logger)
logs.Infow("service started", "name", "api")
```

如需独立实例：

```go
svc := logs.NewService(logger)
svc.Infow("worker ready", "id", 7)
```

## 配置示例

```yaml
logs:
  level: info
  encode: console
  output: console
  levelPattern: /log/level
  levelPort: 22001
```

## 动态级别接口

如无兼容包路径要求，建议直接改用 `logs` 包。

配置了动态级别接口后，可通过 HTTP 调整级别：

```bash
# 查看当前级别
curl http://127.0.0.1:22001/log/level

# 设置为 debug（JSON）
curl -X PUT \
  -H "Content-Type: application/json" \
  -d '{"level":"debug"}' \
  http://127.0.0.1:22001/log/level

# 设置为 warn（表单）
curl -X PUT \
  -d "level=warn" \
  http://127.0.0.1:22001/log/level
```
