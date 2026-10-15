# EduGo

EduGo 是一个模块化单体学习平台：课程、题库、学习记录、考试、复习、计划、证书和通知可以在关闭 AI 时独立运行。

## 运行

1. 安装 Go 1.27+ 与 Node.js 22+。
2. 复制 `configs/config.example.yaml` 为 `configs/config.yaml`。
3. 开发模式（`server.mode=debug`）如果没有配置管理员密码，首次启动会创建默认账号：用户名 `admin`，密码 `admin123`。登录页也会带出这组账号。发布模式不会创建它。

   要改用自己的密码，设置环境变量后再启动。已存在的同名管理员会按这个密码更新：

```bash
export EDUGO_AUTH_JWT_SECRET='请替换为至少32位的随机密钥'
export EDUGO_ADMIN_USERNAME=admin
export EDUGO_ADMIN_PASSWORD='请替换为至少8位的管理员密码'
```

4. 构建并启动：

```bash
bash scripts/build.sh
./bin/edugo
```

开发时也可以先构建前端，再执行 `go run ./cmd/server`。浏览器打开 `http://127.0.0.1:8080`，使用管理员账号登录后创建教师和学生。

健康检查：`GET /healthz`、`GET /readyz`。

## 数据

默认使用 SQLite，文件位于 `data/edugo.db`。SQLite 部署的 `maxOpenConns` 必须为 1。MySQL 与 PostgreSQL 可通过 `EDUGO_DATABASE_DRIVER` 和 `EDUGO_DATABASE_DSN` 切换，Compose 里提供了对应 profile。本仓库的自动化测试目前只在 SQLite 上执行。

发布模式会拒绝过短或占位的 JWT 密钥、演示种子数据和常见弱管理员密码。

## 目录

- `cmd/server`：进程入口
- `internal/app`：依赖装配
- `internal/modules`：业务模块，模块之间只依赖对方的 `api`
- `internal/platform`：配置、数据库、HTTP、事件、调度、查询和模型调用
- `frontend`：Vue 3 页面，构建产物嵌入 `internal/web/dist`

接口约定、取舍和交付范围见 `docs/`。
