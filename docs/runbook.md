# 运行手册

## 启动

设置 `EDUGO_AUTH_JWT_SECRET` 和 `EDUGO_ADMIN_PASSWORD` 后执行 `edugo`。首次启动会迁移表结构，并在管理员密码已配置时创建管理员。

SQLite 文件和上传目录默认在 `data/`。备份时复制数据库文件以及 `data/uploads`。恢复前先停止进程。

## 检查

- `GET /healthz` 返回进程存活。
- `GET /readyz` 在进程开始监听后返回。它目前不单独探测数据库。
- 日志为 JSON，包含 `traceId`。日志不记录密码、令牌和完整 DSN。

## 发布约束

`server.mode=release` 时：

- JWT 密钥至少 32 位，且不能是 `change-me`、`secret` 一类占位值。
- 不能打开演示种子。
- 管理员密码不能是 `admin123`、`password` 等示例口令。
- AI 开启时必须同时提供 `baseURL`、密钥和至少一个聊天模型。`EDUGO_AI_MODEL` 不会写入 embedding profile。

## 容器

```bash
export EDUGO_AUTH_JWT_SECRET='请替换为至少32位的随机密钥'
export EDUGO_ADMIN_PASSWORD='请替换为管理员密码'
docker compose --profile sqlite up --build
```

MySQL、PostgreSQL profile 见 `compose.yaml`。切换驱动需要重启进程，配置不会热更新。
