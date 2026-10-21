# Tolato

用你手边的 AI 助手管理服务器。部署好 Tolato，每台机器装一个小 agent，再往 Claude Code 或 Codex 里贴一段指令 —— 之后「哪台机器磁盘快满了？」这种问题，直接问你的助手就行。它跑的每条命令都有审计记录，危险命令会先停下来等你点头。

服务端不需要配置任何 LLM：用的模型就是你的编码助手本来在用的那个。

> [English](README.md) · 简体中文

## 怎么用

1. **部署 server** —— 一行命令，见 [部署](#部署docker-compose)。
2. **接入节点** —— 登录后点 *添加节点*，把生成的安装命令拿到每台服务器上跑一下。
3. **把 Tolato 装进你的 AI 助手** —— 首页会给你一段指令，贴进 Claude Code、Codex 或任何编码助手，它会：
   - 从你自己的 server（`/skill.md`）拉取 [skill](skills/tolato/SKILL.md)，放进它的 skills 目录；
   - 从你 server 的 release 镜像安装 `tolato` CLI（github.com 不通也能装）；
   - 运行 `tolato auth login`，浏览器里弹出授权页 —— 选只读还是可执行，由你自己点批准。

然后就可以直接用大白话问了：

> 哪几台机器磁盘快满了？
>
> web-01 上的 nginx 一直报 502，帮我看看。
>
> 把这份 nginx.conf 传到 web-01，测一下配置没问题就 reload。

助手在背后调用的是 CLI —— `tolato nodes list`、`tolato exec web-01 -- …`、`tolato cp ./nginx.conf web-01:/etc/nginx/` —— skill 会告诉它什么时候该停下来问你。

## 安全边界

- **权限你来定。** 每次 CLI 登录都是一把独立的 key，只读还是可执行由你在授权页选。所有 key 都能在 Web UI 里查看和吊销，终端里 `tolato auth logout` 也能直接吊销。
- **危险命令要确认。** `rm -rf`、`reboot`、`mkfs` 这类命令不带确认会被拒绝，skill 会让助手先问过你再确认；黑名单里的命令直接拒绝。两份名单都可以在设置里改。
- **全程审计。** 不管从哪个入口执行的命令，都会记进审计日志：谁跑的、从哪儿来的。
- **团队协作。** 用户、用户组、按节点组授权，决定谁能看、谁能操作哪些机器；支持 OIDC 单点登录和用户组同步。
- **多套部署。** CLI 按 server 分 profile，公司和个人的机器可以并存；一旦有多个 profile，skill 会要求助手每条命令都写明用哪个。

## 截图

| 节点列表 | 审计日志 |
|---|---|
| ![节点列表](docs/screenshots/nodelist.png) | ![审计日志](docs/screenshots/auditlog.png) |

## 架构

```
  Claude Code / Codex / …
    └─ tolato CLI（+ skill）── HTTPS /api/v1 ──┐
                                               ▼
┌──────────┐     /api, /ws      ┌──────────┐    /ws/agent    ┌──────────┐
│   web    │ ◄────────────────► │  server  │ ◄─────────────► │  agent   │
│ (Vue 3)  │                    │   (Go)   │                 │   (Go)   │
└──────────┘                    └────┬─────┘                 └──────────┘
                                     │                         被管理节点
                                     ▼
                                ┌─────────┐
                                │Postgres │
                                └─────────┘
```

- **server/** —— Gin HTTP + WebSocket、GORM / Postgres、给 CLI 用的外部 API（`/api/v1`）、权限与审计、探测 / 告警引擎、离线通知、JWT + API-key 认证。同时提供 `/skill.md`，并在 `/releases/` 下镜像 release 资源。
- **agent/** —— 跑在被管理节点上的二进制。命令执行、文件传输、系统指标采集、ICMP / TCP / 带宽探测。通过一次性 token 注册，持久身份写入 `~/.tolato`。
- **cli/** —— 你的 AI 助手调用的 `tolato` 命令：`nodes`、`exec`、`cp`、`auth`、`profile`。
- **skills/** —— 教编码助手安全使用 CLI 的 skill。
- **web/** —— Vue 3 + Vite + shadcn-vue。上手引导、节点、Web 终端、审计日志、用户与权限、设置、可选的网页对话。
- **docs/** —— 设计文档、循环架构、前端架构、NodeProbe、实施计划。

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

打开 `http://localhost:8080`，用 `auth` 里配置的账号登录。首页会一步步带你接入节点、把 Tolato 装进 AI 助手。

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

其余配置 —— 敏感命令规则、离线通知、单点登录、可选的网页对话 LLM —— 都存在数据库里，在 Web UI 的 **设置** 页面修改。

## 其它接入方式

上面的 skill + CLI 是推荐的方式。下面这些用来覆盖它管不到的场景。

### 通过 SSH、没有浏览器

`tolato auth login` 需要同一台机器上有浏览器。在远程机器上，改为在 **设置 → API Keys** 里生成一个 key，用环境变量提供：

```sh
export TOLATO_URL=https://tolato.example.com
export TOLATO_API_KEY=tlat_...
```

### 脚本：直接调 HTTP API

CLI 本身就是 `/api/v1` 下外部 API 的一层薄封装（列出 / 查看节点、执行命令、文件操作）。脚本可以用同样的 key 直接调用，放在 `Authorization: Bearer tlat_...` 头里；key 的权限级别和 CLI 一样生效。

```sh
curl -s https://tolato.example.com/api/v1/nodes \
  -H "Authorization: Bearer tlat_xxxxxxxxxxxxxxxx" | jq
```

### 网页对话（可选）

Web UI 里也内置了一个对话界面，能操作同样的节点，适合手边没有编码助手的时候。这是唯一需要在 server 上配置 LLM 的功能：管理员在 **设置 → LLM 配置** 里填上 OpenAI 兼容的接入点和 key。没配置之前，对话页会提示，并引导你去「接入 AI 助手」。

| 问一句，节点信息直接渲染出来 | 工具调用过程 |
|---|---|
| ![聊天 — 问一句，节点信息直接渲染出来](docs/screenshots/chat_3.png) | ![工具调用过程](docs/screenshots/chat_2.png) |

## 开发

- 编译 server：`cd server && go build ./cmd/server`
- 编译 agent：`cd agent && go build ./cmd/agent`
- 编译 CLI：`go build -C cli -o ~/bin/tolato .`
- 编译 web：`cd web && pnpm build`
- 分支策略：`main` 是发布分支。

## 文档

- [设计总览](docs/design.md)
- [循环架构](docs/loop-architecture.md) —— 网页对话循环的 goroutine + channel 模型
- [前端架构](docs/frontend-architecture.md)
- [NodeProbe](docs/nodeprobe.md) —— 链路监控设计
- [实施计划](docs/implementation-plan.md) —— 按阶段记录进度
