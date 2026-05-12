# Tolato

自然语言服务器运维。在聊天界面里下命令，后端通过 agent 在远端节点上执行命令、采集指标、探测网络链路。

> [English](README.md) · 简体中文

## 截图

![聊天 — 问一句，节点信息直接渲染出来](docs/screenshots/chat_3.png)

| 节点列表 | 审计日志 |
|---|---|
| ![节点列表](docs/screenshots/nodelist.png) | ![审计日志](docs/screenshots/auditlog.png) |
| ![聊天里查 iptables](docs/screenshots/chat_1.png) | ![工具调用过程](docs/screenshots/chat_2.png) |

## 架构

```
┌──────────┐    WebSocket     ┌──────────┐    WebSocket    ┌──────────┐
│   web    │ ◄──────────────► │  server  │ ◄─────────────► │  agent   │
│ (Vue 3)  │   /ws/chat       │   (Go)   │   /ws/agent     │   (Go)   │
└──────────┘                  └────┬─────┘                 └──────────┘
                                   │                         被管理节点
                                   ▼
                              ┌─────────┐
                              │Postgres │
                              └─────────┘
                                   ▲
                                   │   LLM（OpenAI 兼容接口）
                                   ▼
                              聊天循环 + 工具调用
```

- **server/** — Gin HTTP + WebSocket、GORM / Postgres、LLM 聊天循环、工具执行器、会话管理、探测 / 告警引擎、Telegram 通知、JWT + API-key 认证。
- **agent/** — 跑在被管理节点上的二进制。命令执行、系统指标采集、ICMP / TCP / 带宽探测。通过一次性 token 注册，持久身份写入 `~/.tolato`。
- **web/** — Vue 3 + Vite + shadcn-vue。聊天、节点、审计日志、设置、拓扑监控、告警。
- **docs/** — 设计文档、循环架构、前端架构、NodeProbe、实施计划。

## 部署（docker-compose）

仓库自带的 [docker-compose.yaml](docker-compose.yaml) 会从 GHCR 拉取 server 镜像并启动一个 Postgres 容器。Web UI 已经打包进 server 二进制里，无需单独部署。

依赖：Docker + Compose v2。

### 一键安装

```sh
curl -fsSL https://raw.githubusercontent.com/momaek/tolato/main/scripts/install-server.sh | bash
```

会在 `./tolato/` 下拉取 `docker-compose.yaml` + `config.example.yaml`，自动生成随机 `encrypt_key` / `jwt_secret` / admin 密码，然后 `docker compose up -d`。最后会打印登录账号密码 —— **记得保存，只会打印一次。**

常用参数：`--dir <path>` 安装目录、`--port <port>` 对外端口（默认 `8080`）、`--admin-user <name>`（默认 `admin`）。用 `TOLATO_VERSION=v0.1.0` 锁定镜像版本。

### 手动部署

```sh
# 1. 从示例复制出运行时配置。
cp config.example.yaml config.yaml

# 2. 编辑 config.yaml —— 所有 `CHANGE ME` 标记都要改：
#      security.encrypt_key   （32 字节，用于加密数据库里存的敏感字段）
#      security.jwt_secret    （用于签发会话 token）
#      auth.username / auth.password  （Web UI 登录账号密码）
#    另外设置：
#      server.public_address  （外部访问地址，例如 https://tolato.example.com）
#      server.allowed_origins （同一个 URL，用于 CORS 和 WebSocket 来源校验）

# 3. 启动。
docker compose up -d
```

打开 `http://localhost:8080`，用 `auth` 里配置的账号登录。

**锁版本。** compose 文件里用 `${TOLATO_VERSION:-latest}`，生产环境建议固定到具体 tag：

```sh
TOLATO_VERSION=v0.1.0 docker compose up -d
```

**升级。**

```sh
docker compose pull server && docker compose up -d
```

数据库 schema 在启动时自动迁移。

**反代场景**（Caddy / Nginx / Traefik）：在上游终结 TLS，把 `/` 转发到 server 容器的 8080 端口，注意保留 WebSocket upgrade 头。`server.public_address` 要填反代后对外暴露的 URL，这样生成的 agent 安装命令和 WebSocket 地址才是对的。

**Postgres 密码。** compose 和 `config.example.yaml` 里默认都是 `tolato/tolato/tolato`，保持一致。如果改了 compose 里的 `POSTGRES_PASSWORD`，记得把 `config.yaml` 里的 `database.dsn` 同步改掉 —— YAML 配置不支持读 env 变量。

**安装 agent。** 在 Web UI 的 **节点** 页面点 *添加节点*，会生成一次性 token 和 `curl | sudo bash` 安装命令 —— 里面的服务器地址就是 `server.public_address`。

## 本地开发

### 前置依赖

- Go 1.23+
- Node.js 20+ + pnpm
- Docker（跑 Postgres 用）或已有的 Postgres 实例

### 1. 数据库

```sh
docker compose up -d postgres
```

### 2. Server

```sh
cd server
cp config.yaml config.local.yaml   # 跑真实流量前记得改掉默认密钥
go run ./cmd/server -config config.local.yaml
```

默认监听 `:8080`。完整配置参考 [server/config.yaml](server/config.yaml)。

**部署前务必**替换 `security.encrypt_key`、`security.jwt_secret`、`auth.password`；并把 `server.allowed_origins` 设成你前端的域名。

### 3. Web

```sh
cd web
pnpm install
pnpm dev
```

开发服务器跑在 `http://localhost:5173`，API / WS 代理到 `:8080`。

### 4. Agent

在 Web UI 的 **节点** 页面生成一次性注册 token，然后在目标节点上：

```sh
./agent --server ws://your-server:8080/ws/agent --token <one-time-token>
```

Agent 会把身份信息保存到 `~/.tolato/`，后续重启靠它重连，不再需要 token。

跑带宽探测时，在目标节点上另起一个文件服务：

```sh
./agent serve-testfile --port 9090 --size 10
```

## 配置项

### Server（`server/config.yaml`）

| Section    | Key                                      | 作用                                              |
|------------|------------------------------------------|---------------------------------------------------|
| `server`   | `host`、`port`、`allowed_origins`        | 监听地址 + CORS / WS 来源白名单                    |
| `database` | `driver`、`dsn`                          | Postgres 连接串                                   |
| `security` | `encrypt_key`、`jwt_secret`、`agent_token_expiry` | 敏感密钥 —— **必须**替换默认值              |
| `defaults` | `heartbeat_interval`、`command_timeout`、`max_rounds`、`context_rounds`、`output_truncate_lines` | 聊天循环 + agent 调参 |
| `auth`     | `username`、`password`                   | 初始管理员账号（默认 `admin/admin`）              |
| `probe`    | `enabled`、`retention_days`、`telegram`、`alert_rules` | NodeProbe 链路监控配置              |

LLM 的接入点、API key、模型、敏感命令规则、Telegram bot 凭证都存在数据库里，通过 Web UI 的 **设置** 页面配置。

## 开发

- 编译 server：`cd server && go build ./cmd/server`
- 编译 agent：`cd agent && go build ./cmd/agent`
- 编译 web：`cd web && pnpm build`
- 分支策略：`main` 是发布分支。

## 文档

- [设计总览](docs/design.md)
- [循环架构](docs/loop-architecture.md) —— 聊天循环的 goroutine + channel 模型
- [前端架构](docs/frontend-architecture.md)
- [NodeProbe](docs/nodeprobe.md) —— 链路监控设计
- [实施计划](docs/implementation-plan.md) —— 按阶段记录进度
