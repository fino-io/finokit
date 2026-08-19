# errorx

`errorx` 用于在 Go 服务中统一定义、生成和使用业务错误码。

核心流程：

1. 用 YAML 定义业务错误码。
2. 用 `errorxgen` 生成 `errcode` Go 包。
3. 业务代码通过生成的错误定义创建、包装、判断错误。

## 1. 定义错误码

建议把 YAML 放在业务服务内，例如：

```text
configs/error_code/task.yaml
```

示例：

```yaml
appCode: 6
bizCode: 12
errorCode:
  - name: TaskNotFound
    code: 1001
    message: task {task_id} not found
    description: task does not exist
    countInSLA: false

  - name: TaskStateInvalid
    code: 1002
    message: task {task_id} state invalid
    description: task state cannot satisfy current operation
```

字段说明：

- `appCode`：应用编码，范围 `1..9`。
- `bizCode`：业务模块编码，范围 `1..9999`。
- `errorCode`：错误码列表，不能为空。
- `name`：生成后的 Go 变量名，必须是导出的 Go 标识符，例如 `TaskNotFound`。
- `code`：当前业务模块内的错误子码，范围 `1..9999`。
- `message`：错误消息，支持 `{key}` 占位符。
- `description`：生成代码里的注释，可选。
- `countInSLA`：是否计入服务稳定性指标，可选，默认 `true`。

`countInSLA` 用于统一观测层统计错误率、SLA 或告警。通常系统错误、依赖异常、内部处理失败应保持 `true`；参数错误、资源不存在、权限不足、业务状态不允许等预期业务错误可设为 `false`。

完整错误码规则：

```text
appCode(1位) + bizCode(4位) + code(4位)
```

例如：

```text
appCode=6, bizCode=12, code=1001 => 600121001
```

## 2. 生成 errcode 代码

在业务服务仓库中执行：

```bash
go run github.com/fino-io/fino/errorx/cmd/errorxgen \
  -out ./errcode \
  -doc-out ./docs/error-codes.md \
  -pkg errcode \
  ./configs/error_code
```

参数说明：

- `-out`：生成代码目录。
- `-doc-out`：生成的错误码 Markdown 文档路径，可选；所有 YAML 会汇总到同一个文档中，`platform.yaml` 始终排在最前，其它文件按文件名升序排列。
- `-pkg`：生成代码的包名，通常使用 `errcode`。
- `-errorx-import`：`errorx` 的 import path，默认 `github.com/fino-io/fino/errorx`。
- 最后一个参数：YAML 文件或目录。传目录时会递归读取 `.yaml` 和 `.yml` 文件。

本地调试

```bash
go run ./cmd/errorxgen \
  -out "./testdata/errcode" \
  -doc-out "./testdata/error-codes.md" \
  -pkg errcode \
  ./generator/testdata
```

也可以在业务服务中加入 `go:generate`：

```go
//go:generate go run github.com/fino-io/fino/errorx/cmd/errorxgen -out ./internal/errcode -doc-out ./docs/error-codes.md -pkg errcode ./configs/error_code
```

文档为单个 Markdown 文件，包含字段规范、错误码总览和按业务模块拆分的错误码表。`platform` 模块始终排在最前，其它模块按定义文件名升序排列，模块内按错误子码升序排列。未配置 `httpStatus`、`countInSLA` 或 `message` 时，文档会标注运行时实际使用的默认值。

错误码文档遵循以下约定：错误码用于机器判断，`name`（Reason）用于稳定的业务原因标识，`message` 用于面向调用方的可读提示，`httpStatus` 仅表示协议层映射，`countInSLA` 仅表示观测统计策略；运行时 `extra/metadata` 只携带单次错误上下文，不作为错误码定义的一部分。

然后执行：

```bash
go generate ./...
```

生成后的代码大致如下：

```go
var TaskNotFound = errorx.Define(
    600121001,
    "task {task_id} not found",
    errorx.CountInSLA(false),
)

func NewTaskNotFound(opts ...errorx.Option) error {
    return TaskNotFound.New(opts...)
}

func WrapTaskNotFound(err error, opts ...errorx.Option) error {
    return TaskNotFound.Wrap(err, opts...)
}

func IsTaskNotFound(err error) bool {
    return TaskNotFound.Is(err)
}
```

生成时会校验错误名称、业务子码和完整错误码是否重复，冲突会直接终止生成。

## 3. 业务代码中使用

创建业务错误：

```go
return errcode.NewTaskNotFound(
    errorx.WithMessageParam("task_id", taskID),
    errorx.WithExtra(map[string]string{"task_id": taskID}),
)
```

包装底层错误：

```go
if err != nil {
    return errcode.WrapTaskNotFound(err, errorx.WithExtra(map[string]string{
        "task_id": taskID,
    }))
}
```

判断错误类型：

```go
if errcode.IsTaskNotFound(err) {
    // handle task not found
}
```

生成的 `TaskNotFound` 变量保留用于读取错误码元数据；业务代码优先使用 `NewXxx`、`WrapXxx`、`IsXxx`。

在网关、日志、中间件等通用边界读取错误码：

```go
if coded, ok := errorx.From(err); ok {
    code := coded.Code()
    message := coded.Message()
    extra := coded.Extra()
    _, _, _ = code, message, extra
}
```

`err.Error()` 只返回业务消息。JSON 编码会输出业务 `code`、`message`，并在 wrap 了底层错误时输出 `cause`；不会输出 stack。

日志里使用 `%+v` 会输出一行 `code/message/cause`，用于常规错误日志：

```go
logger.Errorf("request failed: %+v", err)
```

示例：

```text
{\"code\":\"100061004\",\"message\":\"get task failed\",\"cause\":\"get task 5: record not found\"}
```

如果确实需要排查调用路径，可对 `*errorx.Error` 显式读取 `StackTrace()`，不要在常规请求日志里默认展开完整 stack。

## 约束

- 同一批 YAML 中不能有重复的错误名。
- 同一批 YAML 中不能有重复的完整错误码。
- YAML 文件名生成 Go 文件名，文件名冲突会生成失败。
- `errorx` 只处理业务错误码，不绑定 HTTP/gRPC 状态码；协议映射应放在业务服务的 adapter 层。
