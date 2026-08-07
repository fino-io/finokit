# Addr

`addr` 包提供一组本机地址相关的辅助函数，用来发现、校验和判断当前机器可用的 IP 地址。

适合这些场景：

- 服务启动时自动选择一个可用地址绑定或上报。
- 从环境变量中读取并校验显式配置的 IP。
- 判断某个 host 或 `host:port` 是否指向本机。
- 在测试或临时服务里申请一个空闲 TCP 端口。

## 快速开始

```go
package main

import (
	"context"
	"log"

	"github.com/fino-io/finokit/addr"
)

func main() {
	ip, err := addr.Primary()
	if err != nil {
		log.Fatal(err)
	}

	loopback, err := addr.Loopback()
	if err != nil {
		log.Fatal(err)
	}

	hostIPv4, err := addr.HostIPv4(context.Background())
	if err != nil {
		log.Printf("hostname lookup failed: %v", err)
	}

	log.Printf("primary=%s loopback=%s hostIPv4=%v", ip, loopback, hostIPv4)
}
```

## 常用函数

### `List()`

返回当前机器所有可用的接口 IP 地址，结果已经做了这些处理：

- 去掉 `0.0.0.0`、`::` 这类未指定地址。
- 去掉组播地址。
- 去重。
- IPv4 会被规范化成 `To4()` 形式。

```go
ips, err := addr.List()
```

### `Primary()`

返回当前机器“最适合对外使用”的地址，优先级如下：

1. 私有 IPv4，例如 `10.x.x.x`、`192.168.x.x`
2. 公网 IPv4
3. 非回环 IPv6
4. IPv4 loopback
5. IPv6 loopback

```go
ip, err := addr.Primary()
```

如果没有可用地址，返回 `addr.ErrNoAddressFound`。

### `Loopback()`

返回本机 loopback 地址，优先 `127.0.0.1`，没有时再退回 `::1`。

```go
ip, err := addr.Loopback()
```

### `HostIPv4(ctx)`

读取当前主机名并通过 DNS/解析器查询对应的第一个 IPv4 地址。

```go
ip, err := addr.HostIPv4(context.Background())
```

适合“机器 hostname 已经正确配置并可解析”的环境；如果查询不到 IPv4，会返回 `addr.ErrNoAddressFound`。

### `FromEnv(key)`

从环境变量中读取一个 IP，并完成空值和格式校验。

```go
ip, err := addr.FromEnv("HOST_IP")
```

常见错误：

- `addr.ErrInvalidEnvKey`：环境变量名为空
- `addr.ErrEnvAddressNotSet`：环境变量未设置或值为空
- `addr.ErrInvalidAddress`：值不是合法 IP

### `IsLocal(value)`

判断一个 host 或 `host:port` 是否指向本机。

支持这些输入：

- `localhost`
- `localhost:8080`
- `127.0.0.1`
- `127.0.0.1:8080`
- `[::1]:8080`

```go
ok := addr.IsLocal("127.0.0.1:8080")
```

说明：

- `localhost` 总是返回 `true`。
- 非 IP 的域名不会做 DNS 解析匹配，例如 `example.com` 会直接返回 `false`。

### `FreeTCPPort()`

在本机 loopback 上临时监听，返回一个当前空闲的 TCP 端口号。

```go
port, err := addr.FreeTCPPort()
```

适合测试、临时 server 或本地调试。

注意：函数返回端口后监听会立即关闭，所以它只能表示“调用时刻空闲”，不能保证后续使用该端口时一定不被抢占。

## 典型用法

### 优先使用显式配置，否则自动探测

```go
ip, err := addr.FromEnv("HOST_IP")
if err != nil {
	if errors.Is(err, addr.ErrEnvAddressNotSet) {
		ip, err = addr.Primary()
	}
}
if err != nil {
	return err
}
```

### 判断回调地址是否指向本机

```go
if addr.IsLocal(callbackAddr) {
	// 走本地优化逻辑
}
```

## 错误约定

包内暴露的常用错误：

- `ErrNoAddressFound`
- `ErrEnvAddressNotSet`
- `ErrInvalidAddress`
- `ErrInvalidEnvKey`

建议调用方使用 `errors.Is` 判断，而不是直接比较错误字符串。
