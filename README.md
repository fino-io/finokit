# finokit

`finokit` 是一组面向 Go 服务的基础设施工具包，提供可复用的配置、日志、数据库、缓存、消息和对象存储能力，帮助业务代码减少对具体基础设施 SDK 的直接依赖。

## 模块

- `config`：配置加载、合并、读取与热更新，支持文件、环境变量、命令行和内存数据源。
- `logs`：兼容封装，统一日志初始化、输出和动态日志级别。
- `db`：基于 GORM 的 MySQL、PostgreSQL 和 SQLite 连接管理。
- `redis`：Redis 客户端与配置封装。
- `messaging`：消息发布/订阅抽象，当前提供 NATS 实现。
- `storage`：对象存储抽象，当前提供 MinIO 和 S3 实现。
- `errorx`：业务错误码定义与 Go 代码生成工具。
- `addr`、`idgen`、`lang/goroutine`：地址、ID 和并发相关辅助工具。

部分模块的详细用法见对应目录下的 README。

## 安装

```bash
go get github.com/fino-io/finokit
```

在代码中按需引入模块：

```go
import "github.com/fino-io/finokit/config"
```

## 开发

项目使用 Go 1.25.12 或更高版本。常用命令：

```bash
make test    # 运行测试并生成覆盖率报告
make lint    # 静态检查
make sec     # 安全扫描
make vuln    # 依赖漏洞扫描
make verify  # 运行 lint、sec 和 vuln
```

生成代码和 mock：

```bash
make mockgen
```
