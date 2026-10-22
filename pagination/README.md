# pagination

通用分页结果、分页参数校验和 AES-256-GCM 游标编解码，不依赖配置、日志、HTTP、gRPC 或数据库。
保留 jsoniter 编码以兼容既有 token 和查询绑定。默认每页 50 条，上限 1000；业务契约可以进一步限制。

```go
codec, err := pagination.NewCursorCodecFromBase64(encodedKey)
// 或传入已解码的 32 字节密钥：pagination.NewCursorCodec(key)
```

配置由应用启动层加载，codec 显式注入仓储。排序字段、查询条件和游标位置由业务仓储解释。
密钥、编码格式和查询绑定保持一致时，兼容原 `gokit/pagination` token。
