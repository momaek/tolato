# Node Agent 离线通知 — 设计方案

> 状态：✅ 已实现（分支 feat/offline-notification）
> 范围：server 端离线检测兜底 + 状态跳变去重 + 多渠道通知（统一 webhook + 预设）+ Web 配置
>
> 实现摘要：
> - 后端 notify 包（Dispatcher / TokenSource / webhook 引擎 / 预设）+ store 去重 + 后台监控，
>   核心模板与 JSON 匹配逻辑有单测（`server/internal/notify/render_test.go`）。
> - 配置存于单个 settings 键 `notify.config`（DB，热生效），REST：`GET/PUT /api/settings/notify`、
>   `GET /api/settings/notify/presets`、`POST /api/settings/notify/test`。
> - Web：`SettingsView.vue` 新增「离线通知」分页（全局阈值 + 渠道列表 + 每渠道测试）。

---

## 1. 背景与现状

Node Agent 通过 WebSocket 连到 server 的 `/ws/agent`，每 30s 发一次 heartbeat
（`agent/internal/client/ws.go:35,417`）。server 收到后 `store.UpdateHeartbeat(nodeID)`
更新 `last_heartbeat` 并置 `status='online'`
（`server/internal/handler/agent_ws.go:277`、`server/internal/store/node.go:129`）。
WebSocket 断开时立即 `SetNodeStatus(nodeID,"offline")`
（`agent_ws.go:139,222`）。状态存 PostgreSQL `nodes` 表的 `status` + `last_heartbeat`
（`server/internal/model/models.go:74-94`）。

### 两个关键缺口

1. **没有超时兜底检测**：只有「WS 干净断开」才置 offline。进程被 kill / 断网 / 宕机时
   TCP 连接半开，server 会长时间误判 online。
2. **完全没有通知机制**：README 提到 Telegram notifier 但代码未实现，无任何
   webhook / 邮件 / IM 告警。

---

## 2. 设计目标

- 补上可靠的离线判定（后台扫描兜底）。
- 状态跳变 **只通知一次**（online→offline 告警，offline→online 恢复）。
- 通知渠道：**统一 webhook 引擎 + 预设**，支持 Telegram / Discord / Slack / 企业微信 / 飞书，
  以及 custom（粘 URL 接任意平台）。
- **多渠道并行**：一次事件发往所有启用的渠道。
- 把「换 token」步骤抽成独立可配置、可复用的单元（TokenSource）。
- 阈值与渠道全部 **DB 落库、Web 可编辑、热生效**（不放 config.yaml）。

---

## 3. 整体架构

```
心跳监控 goroutine ─┐
                    ├─→ store.MarkOffline/MarkOnline(条件 UPDATE, 返回 rowsAffected)
WS 断开/重连 ───────┘            │
                                 └─(仅当真正跳变 rowsAffected==1)→ notify.Dispatcher(event)
                                          ├─→ channel: telegram   (单步)
                                          ├─→ channel: slack       (单步)
                                          └─→ channel: wecom       (两段式, 引用 TokenSource)
```

三条主链路：**检测 → 去重 → 分发**。

---

## 4. 检测：离线兜底扫描

- 在 `server/cmd/server/main.go` 新增 `runOfflineMonitor(rootCtx, settingsCache, dispatcher)`，
  结构照抄现有的 `runGeoIPRefresh`（`main.go:110`）—— ticker + `select{ctx.Done / ticker.C}`。
- 每 tick（建议 = 心跳间隔 30s）扫一次：
  `status='online' AND last_heartbeat < now() - offline_threshold`，逐个走 `MarkOffline`。
- 在 `main()` 启动（`main.go:87-92` 旁）：`go runOfflineMonitor(...)`。

阈值约束：tick 决定检测精度（30s），阈值建议 ≥ 2×心跳（60s），默认 90s；UI 校验最小值，
避免网络抖动误报。

---

## 5. 去重：状态跳变只触发一次（核心）

把「设置状态」改成 **条件更新**，靠 DB 的 `rows affected` 判定真实跳变。
新增 store 函数（替换裸的 `SetNodeStatus`，`server/internal/store/node.go:139`）：

```go
// 仅当当前是 online 才翻成 offline，返回是否发生真实跳变
func MarkOffline(id string) (changed bool, err error) {
    res := DB.Model(&model.Node{}).
        Where("id = ? AND status = ?", id, "online").
        Update("status", "offline")
    return res.RowsAffected == 1, res.Error
}

// 对应 MarkOnline：WHERE status='offline' 时翻回 online（用于恢复通知）
```

收益：
- 监控 goroutine 与 WS 断开处理 **都** 调 `MarkOffline`，谁先到谁触发，
  另一个 `rowsAffected==0` 自动不重复。
- 天然支持多实例 server（DB 行锁兜底）。
- `online→offline` 发离线告警；`UpdateHeartbeat` 里检测 `offline→online` 发恢复通知。

> **防抖修正（重要）**：WS 断开**不再**立即 `MarkOffline`+告警。否则一次网络抖动
> （WS 闪断 2s 又重连）会立刻发一对「离线+恢复」，阈值形同虚设。现在 WS 断开走
> `Dispatcher.OnDisconnect`：监控启用（`offline_threshold_seconds > 0`）时**什么都不做**，
> 把离线判定交给后台监控的阈值——节点心跳静默超过阈值才真正翻 offline，重连刷新心跳即自动
> 吸收抖动。仅当监控被禁用（`<=0`，无兜底探测）时才保留「断开即告警」的旧行为。
> 代价：正常停掉 agent 时，UI 状态会在 ≤ 阈值+一个 tick（默认 ~90–120s）后才显示 offline。
>
> **恢复通知也遵守启动宽限期**：`NotifyOnline` 增加 `inGrace` 判断。否则停机 > 阈值后重启时，
> 监控先把所有节点标 offline（离线告警被宽限期抑制），紧接着 agent 陆续重连触发
> `offline→online`，会发一波「恢复在线」刷屏。

改动点：
- `agent_ws.go:139,222` 的 `SetNodeStatus(...,"offline")` → `MarkOffline(...)`，changed 时 dispatch。
- `store/node.go:130 UpdateHeartbeat`：拆出 `MarkOnline` 或先查旧 status，从 offline 翻 online 时 dispatch 恢复事件。

---

## 6. 分发：统一 webhook + 预设

**所有渠道都是同一种 webhook 引擎**，差别只在「预设」填好的默认值。
不再单独写 Telegram notifier，它退化成「无 token_source 的预设」。

### 6.1 渠道模型

```
WebhookChannel {
  name          string
  enabled       bool
  preset        string   // telegram|discord|slack|wecom|feishu|custom
  tokenSource   string   // 引用 token_sources 里的名字；空=单步发送
  params        map      // 预设占位符的实际值（如 bot_token / chat_id / agentid）
  webhook       struct { method, url, headers, bodyTemplate, successCheck }  // custom 时完整暴露
}
```

- **preset** 决定底层 `url / method / body_template / success_check / invalidate_on` 默认值，
  用户只填 `params`。
- **custom** 暴露完整 webhook 字段，接任何没预设的平台。
- **token_source** 留空=单步（Telegram/Discord/Slack），有值=两段式（企微/飞书）。

### 6.2 各预设填写对照

| 渠道 | token_source | 用户只需填 | 预设自动填 |
|---|---|---|---|
| Telegram | 无 | bot_token、chat_id | sendMessage URL、body |
| Discord | 无 | webhook URL | `{"content":"{{message}}"}` |
| Slack | 无 | webhook URL | `{"text":"{{message}}"}` |
| 企业微信 | 有(gettoken) | corpid、secret、agentid | gettoken/换取、message/send、`errcode 42001` 失效 |
| 飞书/Lark | 有(tenant_access_token) | app_id、app_secret、receive_id | token 换取、`Authorization: Bearer {{token}}`、im/v1/messages |

### 6.3 单步渠道配置示例

Telegram（token 在 URL，chat_id 在 body）：
```yaml
preset: telegram
token_source: ""
method: POST
url: https://api.telegram.org/bot{{bot_token}}/sendMessage
body_template: '{"chat_id":"{{chat_id}}","text":"{{message}}"}'
params: { bot_token: "...", chat_id: "..." }
```

Discord / Slack（token 已在 URL 里）：
```yaml
preset: discord            # 或 slack
token_source: ""
method: POST
url: https://discord.com/api/webhooks/{{id}}/{{token}}   # 也可直接粘整条 URL
body_template: '{"content":"{{message}}"}'                # slack 用 {"text":"{{message}}"}
```

富文本差异（Slack blocks、Discord embeds、Telegram parse_mode）全靠 `body_template`
自由书写，引擎不关心。

---

## 7. TokenSource：独立的「换 token」单元

把换 token 抽成命名、自带缓存/刷新/失效规则的单元，webhook 按名引用，多个渠道可共享同一份缓存 token。

```yaml
token_sources:
  - name: wecom
    request:                 # 怎么换 token（完全可配）
      method: GET
      url: https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid={{corpid}}&corpsecret={{secret}}
      headers: {}
      body: ""
    params: { corpid: "ww...", secret: "..." }   # 敏感，脱敏展示
    extract:                 # 怎么从响应取 token
      token_path: access_token
      expires_path: expires_in       # 秒；缺失则用兜底
      ttl_fallback_seconds: 7000
    invalidate_on:           # 怎么判定失效→作废重取
      json_path: errcode
      values: [42001, 40014]
```

渠道引用：
```yaml
channels:
  - name: wecom-ops
    preset: wecom
    token_source: wecom
    params: { agentid: 1000002 }
    # 预设里：url = .../message/send?access_token={{token}}
```

### TokenSource 职责（一个对象搞定）

- `Get(ctx) (token, error)`：缓存命中直接返回；过期/首次 → 按 `request` 发请求、
  按 `extract` 取 token+TTL、缓存。
- 并发安全（mutex），同源不并发重复换。
- 失效重试钩子：渠道发送命中 `invalidate_on`（或 HTTP 401）→ 作废缓存、重换、渠道重试一次。
  重试只一次，避免和发送重试叠加成多次轰炸。

---

## 8. 配置存储（DB + Web 可编辑）

照 `llm.*` settings 分组的模式（`server/internal/settings/cache.go:108` 的 `Cache.LLM()`），
新增 `notify.*` 分组，全部 DB 落库、UI 可改、热生效。

```yaml
notify:
  offline_threshold_seconds: 90    # 可配，默认 90
  recover_notify: true             # 是否发恢复通知
  token_sources: [ ... ]           # 整个存成 JSON 数组
  channels: [ ... ]                # 多个并行
```

- `Cache.Notify() model.NotifySettings`（仿 `Cache.LLM()`）。
- `model/api.go` 加 `NotifySettings / Channel / TokenSource` 结构（仿 `LLMSettings:195`）。
- `handler/router.go` 加 `GET/PUT /settings/notify` + `POST /settings/notify/test`
  （仿 `router.go:91-101`）。
- 启动时把 `token_sources` 构建成 registry（`map[name]*TokenSource`），渠道按名查找；
  settings 失效钩子触发时重建。

---

## 9. 细节与坑

1. **server 重启惊群**：重启后 DB 里旧节点都是 online 但无连接，监控会在阈值后集中标 offline
   并群发通知。对策：进程记 `startedAt`，启动后一个宽限期（如 1×阈值）内只翻状态、不发离线通知。
2. **异步不阻塞**：通知发送失败（限流/超时）只 log，绝不影响心跳与状态更新。
3. **成功判定各家不同**：Telegram 看 `ok`、Slack 返回纯文本 `ok`、企微看 `errcode`。
   预设里配 `success_check`（类似 `invalidate_on` 的 json_path 判定），留空只看 HTTP 2xx。
4. **失效判定兼容两种**：HTTP 401 + 业务码（如 `errcode`）都要支持。
5. **敏感字段脱敏**：corpid/secret/app_secret/bot_token 在 GET 返回时脱敏
   （仿 `agent_secret` 的 `json:"-"`），UI 展示「已配置/未配置」。
6. **模板替换**：用标准库 `strings.Replacer`（避免 `text/template` 对 JSON 大括号的转义麻烦）。
7. **幂等单测**：`MarkOffline/MarkOnline` 并发双触发只发一次，是核心，值得单测。

---

## 10. 改动文件清单

**新增**
```
server/internal/notify/notify.go        // Dispatcher：遍历 enabled channels，异步发，失败只 log
server/internal/notify/event.go         // Event{NodeName, Alias, IP, Type(offline/online), At, LastHeartbeat}
server/internal/notify/tokensource.go   // TokenSource：Get()+缓存+刷新+Invalidate；registry
server/internal/notify/webhook.go       // 渲染模板 → 发送 → 失效重试一次 → success_check
server/internal/notify/preset.go        // telegram/discord/slack/wecom/feishu 预设默认值
```

**改动**
```
server/internal/store/node.go                  // MarkOffline / MarkOnline
server/internal/handler/agent_ws.go            // 断开处 MarkOffline + dispatch
server/cmd/server/main.go                      // runOfflineMonitor + 启动 dispatcher
server/internal/settings/cache.go              // Cache.Notify()
server/internal/model/api.go                   // NotifySettings / Channel / TokenSource
server/internal/handler/router.go, setting.go  // /settings/notify + /test 端点
web/src/views/SettingsView.vue                 // 通知配置 UI（选预设→填字段→测试）
```

---

## 11. 实现顺序

1. 检测兜底 + `MarkOffline/MarkOnline` 去重（纯后端，单测验证只发一次）。
2. Dispatcher + 统一 webhook 引擎 + 单步预设（Telegram/Discord/Slack）→ 打通链路。
3. TokenSource + 两段式 + 失效重试 + 企微/飞书预设。
4. `/settings/notify` 端点（含 test）+ Web 配置 UI。
