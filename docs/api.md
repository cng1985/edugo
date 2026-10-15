# 接口约定

除健康检查外，业务接口使用 `POST /api/v1/...`，请求和响应均为 JSON。

成功响应：

```json
{"code":200,"msg":"success","data":{},"traceId":"..."}
```

分页响应额外包含 `page`、`size`、`total`、`responseType`。失败响应使用 HTTP 状态码，并带 `errorCode` 与 `msg`。响应头 `X-Trace-Id` 与 `traceId` 相同。

登录接口为 `POST /api/v1/auth/login`。其他业务接口需要 `Authorization: Bearer <accessToken>`。开始考试必须带 `Idempotency-Key`。

AI 辅导在客户端声明 `Accept: text/event-stream` 时返回 SSE，事件顺序为 `started`、`delta`、`completed` 或 `error`。未声明该头时返回普通 JSON。

通用查询为 `POST /api/v1/query/search|list|one`。模型、字段、操作符和排序必须命中白名单。`one` 没有命中时返回 `code=200` 且 `data=null`。

证书核验 `POST /api/v1/certificate/verify` 不需要登录。
