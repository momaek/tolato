# Tolato 多租户改造评估

> 状态：**评估中，未决策**。本文用于判断是否要做多租户，以及若做，工作量和关键决策点是什么。
> 撰写日期：2026-06-07。基于 commit `6d762c5` 时的代码。

## 1. 一句话结论

Tolato 当前是**严格的单用户架构**，对多租户「零支持」但也「零坏债」。是否改造**不是技术问题，是产品定位问题**：

- 自托管、一组织一实例 → **不要做**，现状够用，多部署实例比多租户更简单可靠。
- 要做成 SaaS（你托管，多个互不信任客户共用一套）→ 多租户是前提，是 2–3 周的系统工程。
- 中间态（自托管但要多用户登录、审计归属）→ 轻量改造，约 1 周，不需要完整租户隔离。

## 2. 现状盘点

### 2.1 认证

- 配置文件硬编码 `username/password`（默认 `admin/admin`）—— [config.go](../server/internal/config/config.go) `AuthConfig`
- 登录直接字符串比对 —— [auth.go](../server/internal/handler/auth.go)
- JWT Claims 只含 `Username`，有效期固定 24h；`JWTAuth()` 只验签，**从不用 username 做数据隔离** —— [middleware/auth.go](../server/internal/middleware/auth.go)
- 前端只存单个 token，无用户 ID 概念 —— [stores/app.ts](../web/src/stores/app.ts)

### 2.2 数据模型（全部无归属字段）

模型定义见 [models.go](../server/internal/model/models.go)，ORM 为 GORM + PostgreSQL，auto-migration。

| 表 | 归属字段 | 说明 |
|----|---------|------|
| `conversations` | ❌ 无 | 任何 token 看到全部对话 |
| `messages` | ❌ 无 | 随 conversation 走 |
| `nodes` | ❌ 无 | 全局共享节点池 |
| `registration_tokens` | ❌ 无 | 一次性注册 token，不绑用户 |
| `audit_logs` | ⚠️ 仅 `api_key_id` | 能追到是哪个 key，但日志全局可见 |
| `settings` | ❌ 无 | 全局 KV（LLM key、确认开关等） |
| `api_keys` | ❌ 无 | 全局权限级别 readonly/standard/admin，无 owner |

### 2.3 Store / Handler 层

- `ListConversations` / `ListNodes` 直接返回全部，无 `WHERE user_id` —— [store/conversation.go](../server/internal/store/conversation.go)、[store/node.go](../server/internal/store/node.go)
- `GetConversationByID` 只要知道 ID 谁都能取
- Node 由 agent 凭一次性 token 注册创建，`agent_secret` 仅用于重连鉴权，**不是**租户隔离手段 —— [agent_ws.go](../server/internal/handler/agent_ws.go)、[node/manager.go](../server/internal/node/manager.go)

### 2.4 风险结论

- ❌ 零用户隔离：任何有效 token 可访问全部数据
- ❌ 零 RBAC：无角色 / 资源级权限
- ❌ 零数据分隔：单库混合
- ✅ 但架构干净，无「埋了 user_id 却不检查」的半成品坑，改造起点清晰

## 3. 三档方案对比

### 方案 A — 维持单用户（推荐，若自托管）

不动任何代码。需要多组织隔离时多部署实例。
**工作量：0。** 风险最低，把精力留给核心体验（如近期的 agent 自更新、终端重连）。

### 方案 B — 轻量多用户（若自托管但要团队协作）

同一实例多个成员登录，资源默认互相可见，只做「账号体系 + 审计归属」，**不做**完整租户隔离。

改动：
- `Auth` 配置从单账号改为用户表（或配置多账号）
- JWT 加 `user_id`，`JWTAuth()` 写入 context
- `audit_logs`、`conversations` 加 `user_id` 仅用于「谁做的」展示，不做访问拦截
- API Key 绑定创建者

**工作量：约 1 周。** 不触碰 Node 归属、Settings 拆分等重活。

### 方案 C — 完整 SaaS 多租户（若要对外托管）

多个互不信任租户共用一套，需真正的数据隔离。

必须改：

| 层 | 改动 | 量级 |
|----|------|------|
| Model | 各表加 `user_id`/`tenant_id` FK | 🔴 |
| Store | 所有查询加 `WHERE` 过滤 + 权限校验 | 🔴 ~200–300 行 |
| Handler | 从 JWT 取 user 传入 store，逐接口加鉴权 | 🔴 ~300–400 行 |
| Middleware | `JWTAuth` 注入 user_id，可选 RBAC 中间件 | 🟡 ~50–100 行 |
| Settings | 拆 `global_settings` + `user_settings`（LLM key 等谁出） | 🔴 设计决策重 |
| API Key | 绑 owner + 继承权限 | 🟡 |
| Agent 注册 | 注册 token 编码 tenant，注册时绑定 Node 归属 | 🟡 |

**工作量：约 2–3 周**（含测试与数据迁移脚本），具体看 Settings / API Key 处理深度。

Agent 二进制本身基本不用改——租户归属在 server 端处理，token 里带上 tenant 即可。

## 4. 若走方案 C 的关键决策点

1. **Node 所有权模型**
   - 一对一（Node 属单租户）：简单，**推荐起步**
   - 多对多（Node 可被多租户共享）：需新表 `user_node_permissions`，灵活但复杂

2. **Settings 处理**
   - 全局 + 用户级覆盖：复杂度中等，LLM key 可平台兜底
   - 彻底拆分：每租户自带 LLM key，部署/引导成本高

3. **API Key 权限**
   - 绑 owner 继承权限：最小改动，**推荐**
   - 资源级细粒度（key 只读 nodeA）：+约 1 周

## 5. 建议路径

1. 先定产品方向（A/B/C），再动手。
2. 若不确定要不要 SaaS，但想团队多人用 → 走 **方案 B**，它是 C 的子集，不会白做。
3. 真要 SaaS → 从 C 的「一对一 Node + owner 继承 API Key」最小集起步，后续再升级共享与细粒度权限。

## 6. 改造优先级文件清单（方案 C）

1. [models.go](../server/internal/model/models.go) — 加 user_id FK
2. [store/conversation.go](../server/internal/store/conversation.go) — 加 WHERE 过滤
3. [store/node.go](../server/internal/store/node.go) — 加权限过滤
4. [handler/conversation.go](../server/internal/handler/conversation.go) — 传 user_id
5. [handler/node.go](../server/internal/handler/node.go) — 传 user_id
6. [middleware/auth.go](../server/internal/middleware/auth.go) — 注入 user_id 到 context
7. [config.go](../server/internal/config/config.go) — Auth 升级为多用户
