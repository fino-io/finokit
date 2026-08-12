# Config

`config` 包提供统一的配置加载、合并、读取与热更新能力，支持多数据源（文件、环境变量、命令行、内存等）。

## 快速开始（推荐）

应用启动时先初始化默认配置，再加载配置源：

```go
package main

import (
	"log"

	"github.com/fino-io/finokit/config"
)

type AppConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

func main() {
	// 1) 初始化默认配置实例
	if err := config.InitDefault(); err != nil {
		log.Fatal(err)
	}

	// 2) 加载目录（会递归读取 json/yaml/xml）
	// 支持 conf/ config/ configs/ 或自定义路径
	if err := config.LoadPath("./configs"); err != nil {
		log.Fatal(err)
	}

	// 3) 按结构体读取
	var app AppConfig
	if err := config.ScanFrom(&app, "app"); err != nil {
		log.Fatal(err)
	}

	log.Printf("listen on %s:%d", app.Host, app.Port)
}
```

## 常用加载方式

```go
// 单文件
_ = config.LoadFile("./configs/config.yaml")

// 目录递归加载
_ = config.LoadPath("./configs")

// 按后缀环境筛选（示例：*.prod.yaml）
_ = config.LoadPathWithSuffix("./configs", "prod")

// 命令行参数（需先 flag.Parse()）
_ = config.LoadFlag()

// 加载当前运行目录的 .env，并探测 conf/config/configs（可配 CONFIG_PATH）
_ = config.LoadDefaultSources()
```

## 读取配置

```go
// 点路径或分段路径都可以
v, _ := config.Get("db.host")
host := v.String("127.0.0.1")

v2, _ := config.Get("db", "port")
port := v2.Int(3306)

// 整体扫描
var c map[string]any
_ = config.Scan(&c)

// 带兜底 key 的扫描
var logLevel string
_ = config.ScanFrom(&logLevel, "projectLogs.level", "log.level", "level")
```

## 热更新监听

监听某个路径变更：

```go
w, err := config.Watch("projectLogs.level")
if err != nil {
	// handle
}
defer w.Stop()

for {
	val, err := w.Next()
	if err != nil {
		break
	}
	_ = val.String("info")
}
```

或使用回调：

```go
import "github.com/fino-io/finokit/config/reader"

closer, err := config.WatchFunc(func(v reader.Value) {
	// 配置变化时回调
}, "projectLogs.level")
if err == nil {
	defer closer.Close()
}
```

## 不使用全局默认实例

如果不希望使用包级全局状态，可自行创建实例：

```go
import "github.com/fino-io/finokit/config/source/file"

cfg, err := config.NewConfig(
	config.WithSource(file.NewSource(file.WithPath("./configs/config.yaml"))),
)
if err != nil {
	// handle
}

v, _ := cfg.Get("app.name")
_ = v.String("default")
```

## 可选项（Option）

- `WithLoader(l loader.Loader)`: 自定义 loader
- `WithReader(r reader.Reader)`: 自定义 reader
- `WithSource(s source.Source)`: 初始化时追加数据源
- `WithWatcherDisabled()`: 禁用 watcher（适合 CLI/测试场景）

## 注意事项

- 使用包级 API（`Get/Scan/Watch/Sync`）前，建议先调用 `InitDefault` 或显式 `Load*`。
- `Get` 路径支持两种形式：`"a.b.c"` 与 `"a", "b", "c"`。
- `Map()`、`Bytes()` 可用于调试当前生效配置快照。
- `Sync()` 可强制从 source 重新拉取并刷新内存视图。
