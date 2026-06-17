# Tolato 安全深度研究报告：WebSSH / Agent 控制面

> 日期：2026-06-17
> 分支：`secrity-check`
> 范围：`server/`、`agent/`、`web/`，重点为 WebSSH/Terminal、Agent WebSocket、命令执行、文件操作、MCP/LLM 工具链、部署与更新链。
> 方法：5 轮 deep-investigate 多代理静态代码审计 + 外部 WebSSH/WebSocket/远程访问网关资料核查。
> 重要限制：本次未启动生产服务、未连接真实 agent、未进行破坏性 DoS/文件读写/终端攻击测试，未读取或披露运行时数据库、WAL、SHM、identity、token 或其他真实 secret。风险等级基于代码路径、权限前提、影响面和行业攻击模型评估。

---

## 1. 执行摘要

Tolato 当前实现更接近 **单管理员、单租户、全局控制面**，而不是多用户、多租户平台。一个有效管理员 JWT 代表全局控制能力：节点管理、设置修改、API key 管理、Chat/MCP/External API 命令执行，以及 `/ws/terminal` WebSSH/文件操作能力。

本次研究的核心结论是：

1. **WebSSH / Terminal 是最高优先级风险面。** `/ws/terminal` 在首帧 JWT 认证后可按 `node_id` 打开任意在线 agent 的 PTY，并转发浏览器输入；该通道绕过普通命令执行路径中的敏感命令确认和黑名单策略。`file_op` 还提供 agent 主机权限范围内的文件 list/stat/read/write/mkdir/delete 能力。
2. **Agent 是高权限执行平面。** agent 可执行 shell 命令、启动 PTY、读写文件、自更新。若默认 root/systemd 运行或运行在高权限账户下，控制面失陷会扩展为节点主机高权限控制。
3. **Agent 身份、注册 token 和更新链需要优先加固。** agent 重连凭据为 `node_id + secret`，通过 URL query 使用；registration token 默认可长期/多次使用；agent update 下载二进制后仅以 `--version` 做弱校验，未见签名或 checksum manifest 校验。
4. **API/MCP/Chat 工具链都触达远程执行面。** External API、MCP、Chat tool 都可执行命令或触发 agent update，但权限粒度、确认机制和审计不一致；API key 只有 `readonly / standard / admin`，缺少 node/capability scope。
5. **审计与资源边界不足。** 当前 AuditLog 更像 command history：terminal 只记录 `[terminal session]`，`file_op`、agent reconnect、update、settings/API key 变更等关键安全事件缺少完整审计。WebSocket、MCP body、PTY session、file_op、command output 等缺少统一 size/rate/concurrency 边界。
6. **前端 XSS 未被动态证实，但影响放大链明显。** JWT 存于 `localStorage`，前端有 Markdown/xterm/link 渲染面，且代码中未见 CSP 等安全响应头。若任一渲染链 XSS 成立，可读取 JWT 并升级为 WebSSH 控制节点。

### 最重要的 P0 整改

- 为 `/ws/terminal` 与 `file_op` 增加能力 token、节点/操作授权、资源限制和审计；默认禁用或限制文件写/删。
- registration token 改为默认短 TTL、一次性或有限次数；agent secret 不再以 query bearer 形式长期使用，支持哈希存储、轮换、重连异常审计。
- agent update 引入 signed manifest / checksum / 公钥验证，`update_agent` 纳入确认与审计。
- 清理仓库中运行态 DB/WAL/本地配置的跟踪状态并轮换可能泄露的凭据；settings secret 加密落库。
- API key 增加 scopes、TTL、node allowlist；MCP/External API/Chat tool 统一走策略引擎。

---

## 2. 研究方法与验证边界

### 2.1 研究轮次

- 第 1 轮：server / agent / web 静态审计，识别 WebSSH 主链、agent 执行面、前端 token/XSS 面。
- 第 2 轮：外部 WebSSH/WebSocket 威胁情报、协议路径代码验证、部署默认与 secret 管理。
- 第 3 轮：MCP/LLM tool/prompt injection、前端 source→sink、审计取证/告警。
- 第 4 轮：恶意 agent 反向攻击模型、DoS/资源耗尽模型、身份/RBAC 边界。
- 第 5 轮：权限矩阵、Top 风险、整改路线图、证据与外部引用核查。

每轮均由独立 verifier 审计，前 4 轮输出 GAPS，第 5 轮 PASS。

### 2.2 已代码级验证

- JWT 仅含 `username`，无 role/tenant/node scope。
- `/ws/terminal` 首帧 JWT 后可按 `node_id` 打开在线 agent PTY。
- terminal input 直接进入 agent PTY。
- terminal `file_op` 可转发到 agent 本地文件操作；agent 文件模块未见路径沙箱。
- External API / MCP / Chat tool 都能触达命令执行或 agent update。
- command sensitive/blacklist 不覆盖 raw terminal PTY。
- agent reconnect 使用 `node_id + secret`，同 node 新连接替换旧连接。
- registration token 默认可永不过期且可复用。
- AuditLog 对 terminal/file_op/update/agent reconnect 覆盖不足。
- JWT 存 `localStorage`，前端存在 Markdown/v-html/xterm/CSP 防御面。

### 2.3 未动态实测，不应过度断言

- 未证明具体 XSS payload 可执行。
- 未证明 prompt injection 可稳定越过用户确认。
- 未实际读取/写入任何敏感文件。
- 未实际执行 terminal 命令 PoC。
- 未做 WS/PTY/file_op/command DoS 压测。
- 未模拟恶意 agent 抢占连接。
- 未验证真实部署 TLS/反代/CSP/security headers。
- 未读取本地 DB、secrets、runtime identity 文件内容。
- 未动态验证 agent update 攻击链。

---

## 3. 外部资料与行业基线

### 3.1 OWASP WebSocket Security Cheat Sheet

OWASP 指出 WebSocket 主要风险包括 CSWSH、认证绕过、注入、DoS 和监控盲区，并强调：

- 每次握手都应校验 `Origin`，使用显式 allowlist。
- 不应假设 WebSocket 连接认证后即可无限制访问所有 action；每条消息都应做授权。
- WebSocket 隧道（SSH/VNC/FTP 等）需要额外认证与访问控制。
- 需要 message size limit、rate limit、idle timeout、heartbeat、backpressure、事件日志。

参考：<https://cheatsheetseries.owasp.org/cheatsheets/WebSocket_Security_Cheat_Sheet.html>

Tolato 已有 WebSocket Origin 检查框架，但 `/ws/terminal` 的风险重点在：认证后缺少 node/capability 级授权、资源边界和操作级审计。

### 3.2 Gitpod CSWSH / CVE-2023-0957

Gitpod 曾因 WebSocket Origin 未限制导致攻击者可用受害者凭据建立 WebSocket，最终可能接管 workspace。该案例说明：浏览器 WebSocket 一旦通向远程执行/工作区控制面，Origin、会话与消息级授权错误会导致高影响接管。

参考：

- <https://nvd.nist.gov/vuln/detail/CVE-2023-0957>
- <https://github.com/advisories/GHSA-f53g-frr2-jhpf>

注意：Tolato 不是同一漏洞类型。Tolato 目前使用首帧 JWT 而非 cookie 自动携带，且已有 Origin 检查；该案例用于风险模式类比，不证明 Tolato 存在同一 CSWSH。

### 3.3 Apache Guacamole / 远程访问网关反向输入风险

Apache Guacamole 的历史 CVE（例如 CVE-2023-43826）说明远程访问网关必须把被连接目标也视为不可信输入源。恶意/被控远端可通过协议数据反向攻击网关进程。

参考：<https://nvd.nist.gov/vuln/detail/CVE-2023-43826>

映射到 Tolato：恶意 agent 可回传 register/heartbeat/command_result/pty_output/file_result/update_result，污染控制面状态、审计、UI 与 LLM 上下文。Tolato 的风险不是同类内存破坏，而是信任边界、资源边界和输出污染。

### 3.4 软件更新完整性 / TUF

The Update Framework（TUF）强调用签名元数据、版本/回滚保护等机制保护软件更新系统，即使仓库或部分密钥受损也能降低风险。

参考：<https://theupdateframework.io/>

Tolato 不一定需要完整引入 TUF，但 agent update 至少应具备 signed manifest、artifact digest、公钥校验和回滚防护。

### 3.5 对照 Nezha（哪吒监控）实际 CVE 的核验

Nezha 是与 Tolato 架构同类的开源监控/agent 控制面（dashboard + agent + WebSSH + 命令下发）。2025 年底起 Nezha 一方面被实际用作后渗透 RAT（安全厂商对受害者的头号建议是「务必关闭 Web SSH 功能」），另一方面被披露一连串高危 CVE。这些真实漏洞可作为 Tolato 威胁模型的**事后验证基线**。本节据此逐项核验 Tolato 代码（2026-06-17，代码级静态核验）。

| Nezha 漏洞 | 根因 | Tolato 对应处 | 核验结论 |
|---|---|---|---|
| 实际攻击：WebSSH 被当作隐蔽 RAT 滥用 | Web SSH 功能本身被滥用 | `/ws/terminal` | **印证 §5.1**：WebSSH 是头号风险面（P0） |
| CVE-2026-46716（9.9 跨租户 RCE） | cron 路由挂在 `commonHandler`（任意登录用户）而非 `adminHandler`，叠加权限恒真绕过 → 给全局 ServerShared 每台机器下发命令 | `router.go` 路由分组 | **不存在**：敏感路由均在 `protected` + `JWTAuth()` 组（`router.go:72-125`）；JWT 为 all-or-nothing 管理员，无 member 档可错放 |
| CVE-2026-53519（9.1 未认证任意文件读） | 前端 fallback 用 `strings.HasPrefix(path,"/dashboard")` 子串匹配 → `/dashboard../etc/passwd` 穿越，且从真实 OS 文件系统取文件 | `webui.go` NoRoute SPA fallback（`webui.go:34-51`） | **不受影响**：服务对象是 `embed.FS`（仅含前端打包产物，碰不到宿主文件）；`http.FileServer` + `io/fs.ValidPath` 拒绝含 `..` 的路径；此处 `HasPrefix` 仅用于把 `/api/`、`/ws/` 排除出 fallback，并非文件访问边界 |
| CVE-2026-46717（RoleMember 可达 SSRF） | notification 路由挂错 handler，反射内网响应体 | `release.go` ReleaseProxy（无认证 proxy，`release.go:20-57`） | **基本安全**：target host 被固定 upstream 前缀钉死，`*path` 只能往 path 拼，改不了 host/scheme；遗留轻微带宽/DoS 面，归入 §5.9 |
| CVE-2026-48119（agent 伪造他人 service 结果） | 缺少 agent→service 归属校验 | agent 回包处理 | 单租户模型下不构成「跨租户」伪造；节点冒充仍受 §5.5 约束（需目标 node secret） |
| CVE-2026-47124（WS 跨租户遥测泄露） | WS 认证后缺消息级授权 | WS `CheckOrigin`（`middleware/origin.go:24-46`） | CheckOrigin 用 `EqualFold(u.Host, r.Host)` **精确比较**，非子串；单租户下无跨租户数据隔离需求 |

核验副产物：在比对「子串匹配替代严格校验」这一根因模式时，发现 HTTP CORS 同源判断存在同类子串 bug，详见 §5.11。

**总体判断**：Tolato 在 Nezha 实际被攻击/披露的几个最危险点（未认证文件读、路由授权、CSWSH origin、跨租户 RCE）上反而是干净的，安全基线优于 Nezha 当时。核心待加固项仍为本报告原有结论中的 WebSSH/`file_op` 能力面（高权限、弱护栏、无审计），且其前置条件为有效管理员 JWT——属「待加固」而非「未授权可打」。

参考：

- <https://www.ithome.com.tw/news/171636>
- <https://siliconangle.com/2025/12/22/ontinue-warns-attackers-abusing-nezha-monitoring-tool-stealthy-remote-access-trojan/>
- <https://advisories.gitlab.com/golang/github.com/nezhahq/nezha/CVE-2026-46716/>
- <https://www.thehackerwire.com/nezha-monitoring-unauthenticated-file-read-cve-2026-53519/>
- <https://advisories.gitlab.com/golang/github.com/nezhahq/nezha/CVE-2026-48119/>

---

## 4. 权限与能力矩阵

| 凭据/入口 | 认证位置 | 主要能力 | 当前约束 | 风险定性 |
|---|---|---|---|---|
| JWT | API Bearer；Chat/Terminal WS 首帧 | 管理全部节点、settings、API key、audit；打开 WebSSH；触发 update | 24h；无 role/tenant/node scope | 当前单管理员：高 blast radius；多用户：Critical |
| API key `readonly` | `/api/v1`、`/mcp` | list/get nodes；MCP web_fetch | 不能命令/edit | 全局资产信息暴露面 |
| API key `standard` | 同上 | execute command；MCP edit/execute/web_fetch | 敏感命令需 `confirm:true`；无 node scope | High，泄露后全局命令能力 |
| API key `admin` | 同上 | execute command，敏感命令免确认 | blacklist 仍生效；无 node scope | High/Critical |
| MCP client | `/mcp` + API key | list/get/edit nodes、execute_command、web_fetch | 依赖 API key 三档权限 | 外部 LLM/自动化放大风险 |
| Chat LLM tool | WebUI JWT 会话 | list/get/edit/execute_command/update_agent/web_fetch | execute_command 才有 sensitive confirmation | prompt injection / tool policy 风险 |
| Agent secret | `/ws/agent?node_id=&secret=` | 冒充/重连指定节点；接收命令/PTY/file/update | 无 mTLS/设备绑定；新连接替换旧连接 | 节点身份劫持风险 |
| Registration token | `/ws/agent?token=` | 注册新 node/agent | 默认可永不过期、可复用 | 恶意 agent 注册/资产污染 |
| Terminal session | `/ws/terminal` JWT 后 | PTY input/output；file_op | 无短期 capability token；无 file sandbox | Critical 管理能力 |

---

## 5. Top 风险与证据

### 5.1 WebSSH terminal 获得完整交互式主机控制，绕过 command guard

- 严重性：Critical
- 优先级：P0
- 前置条件：有效 JWT；目标节点在线。
- 影响：打开任意在线节点 PTY，输入任意命令；绕过普通命令路径的 sensitive confirmation / blacklist；若 agent 高权限运行，可导致节点高权限控制。
- 证据：
  - Terminal WS 首帧认证：`server/internal/handler/terminal_ws.go:34-66`
  - open 只检查 `node_id`、在线连接和 DB 节点：`server/internal/handler/terminal_ws.go:68-97`
  - 打开 agent PTY stream：`server/internal/handler/terminal_ws.go:108-121`
  - input 直接转发：`server/internal/handler/terminal_ws.go:171-176`
  - agent 创建 PTY：`agent/internal/client/ws.go:447-467`
  - Unix shell 启动：`agent/internal/terminal/pty_unix.go:15-35`
  - command guard 只覆盖 External API/MCP/Chat command：`server/internal/handler/external.go:116-133`、`server/internal/mcp/tools.go:262-275`、`server/internal/agent/tools.go:181-207`
- 验证状态：代码级验证；未动态执行 terminal PoC。

### 5.2 `file_op` 对 agent 主机任意路径读/写/删，无路径沙箱

- 严重性：Critical
- 优先级：P0
- 前置条件：有效 JWT + terminal session。
- 影响：读取敏感文件、写入持久化、删除关键文件；影响取决于 agent 运行用户权限。
- 证据：
  - Terminal 接收 `file_op` 并并发处理：`server/internal/handler/terminal_ws.go:186-190`
  - 原样转发 Op/Path/Data/Mode/Offset/Length：`server/internal/handler/terminal_ws.go:229-257`
  - agent 文件操作只检查 path 非空：`agent/internal/files/files.go:56-79`
  - read：`agent/internal/files/files.go:118-160`
  - write：`agent/internal/files/files.go:164-188`
  - mkdir/delete：`agent/internal/files/files.go:190-205`
- 验证状态：代码级验证；未读取/写入真实敏感文件。

### 5.3 JWT 是全局控制面凭据，缺少角色/租户/节点级授权

- 严重性：当前单管理员 High；未来多用户 Critical
- 优先级：P0/P1
- 证据：
  - JWT claim 只有 username：`server/internal/middleware/auth.go:16-20`
  - token 只写 username/exp/issuer：`server/internal/middleware/auth.go:22-39`
  - JWT 中间件只设置 username：`server/internal/middleware/auth.go:63-78`
  - Protected group 覆盖 nodes/settings/audit/API keys：`server/internal/handler/router.go:71-123`
- 定性：当前不是传统 IDOR，而是单管理员全局权限模型；若支持多人/团队，则是系统性 broken access control。

### 5.4 API key 权限过粗，无 node/capability scope

- 严重性：High/Critical
- 优先级：P0
- 证据：
  - APIKey 只有 permission：`server/internal/model/models.go:134-143`
  - 创建只校验三档：`server/internal/handler/apikey.go:47-65`
  - External API readonly 禁止，其余可执行：`server/internal/handler/external.go:97-146`
  - MCP 同样使用三档 permission：`server/internal/mcp/tools.go:18-20`、`server/internal/mcp/tools.go:258-285`
- 风险：standard/admin key 泄露即可对所有节点执行命令；`confirm:true` 是调用方自声明，不是强审批。

### 5.5 Agent secret 可用于节点冒充/连接替换

- 严重性：High
- 优先级：P0/P1
- 证据：
  - reconnect 使用 query `node_id` + `secret`：`server/internal/handler/agent_ws.go:75-86`
  - 新连接替换旧连接：`server/internal/node/manager.go:277-297`
  - agent 连接 URL 中带 secret：`agent/internal/client/ws.go:276-285`
- 风险：secret 泄露后可伪装节点、接收命令、伪造结果、污染状态；未动态实测抢占。

### 5.6 Registration token 默认永不过期且可复用

- 严重性：High
- 优先级：P0/P1
- 证据：
  - CreateNode 返回 token 和 install command：`server/internal/handler/node.go:44-76`
  - 默认 `AgentTokenExpiry: 0`：`server/internal/config/config.go:97-100`
  - 非正 expiry 设为 9999 年：`server/internal/store/node.go:13-27`
  - 模型注释 token 可注册多个节点：`server/internal/model/models.go:98-100`
- 风险：泄露 install command 后可注册恶意 agent，污染资产列表并反向攻击控制面。

### 5.7 Agent update 缺少端到端完整性校验

- 严重性：High；供应链被污染时 Critical
- 优先级：P0/P1
- 证据：
  - Web API 可触发 update：`server/internal/handler/node.go:282-323`
  - Chat tool 可触发 `update_agent`：`server/internal/agent/tools.go:121-135`、`server/internal/agent/tools.go:432-449`
  - NodeManager 发送 update 并信任 agent 回包：`server/internal/node/manager.go:383-407`
  - agent 下载 release asset：`agent/internal/client/update.go:118-174`
  - 只执行 `--version` 验证：`agent/internal/client/update.go:91-99`、`agent/internal/client/update.go:176-188`
  - 原子替换：`agent/internal/client/update.go:107-115`
  - ReleaseProxy 只流式代理：`server/internal/handler/release.go:20-57`
- 注意：不能写成 ReleaseProxy 已导致 RCE；应写成缺少签名/hash/TUF 类完整性防线。

### 5.8 审计覆盖不足，terminal/file_op/update/agent reconnect 不可追溯

- 严重性：High
- 优先级：P1
- 证据：
  - AuditLog 字段偏 command history：`server/internal/model/models.go:109-122`
  - terminal 只写 `[terminal session]`：`server/internal/handler/terminal_ws.go:211-225`
  - file_op 只回传浏览器，无审计：`server/internal/handler/terminal_ws.go:229-257`
  - WebUI/API/MCP command 写 stdout/stderr：`server/internal/agent/tools.go:397-422`、`server/internal/handler/external.go:149-177`、`server/internal/mcp/tools.go:287-310`
- 风险：攻击后无法还原 WebSSH 输入、文件路径、agent 替换、update 触发者；同时 stdout/stderr 审计可能成为 secret 二次泄露面。

### 5.9 WebSocket、MCP、PTY、file_op、command output 缺少资源边界

- 严重性：Medium/High
- 优先级：P1
- 证据：
  - Terminal auth 后清空 deadline，未见 heartbeat/read limit：`server/internal/handler/terminal_ws.go:43-66`
  - AgentConn 持续 ReadMessage，未见 ReadLimit：`server/internal/node/manager.go:188-219`
  - file_op 每条 goroutine：`server/internal/handler/terminal_ws.go:186-190`
  - MCP `io.ReadAll` 全量读 body：`server/internal/mcp/server.go:45-55`
  - MCP batch 无 item cap：`server/internal/mcp/server.go:73-90`
  - command stdout/stderr 全量 buffer：`agent/internal/executor/command.go:60-70`
- 验证状态：代码级边界缺失；未做 DoS 压测。

### 5.10 前端 JWT localStorage + 富文本/xterm 渲染防御纵深不足

- 严重性：Medium/High（影响高，直接可利用性未验证）
- 优先级：P1
- 证据：
  - token 存 localStorage：`web/src/stores/app.ts:7-20`
  - API 使用 token：`web/src/services/api.ts:40-45`
  - terminal WS 首帧发 token：`web/src/services/terminalWs.ts:34-40`
  - Markdown 渲染：`web/src/components/chat/ContentBlock.vue:1-15`
  - CodeBlock `v-html`：`web/src/components/chat/CodeBlock.vue:18-28`、`web/src/components/chat/CodeBlock.vue:97-98`
  - xterm WebLinksAddon：`web/src/views/NodeTerminalView.vue:7`、`web/src/views/NodeTerminalView.vue:121-122`
  - 未见统一 CSP/HSTS/X-Frame-Options 等安全 header。
- 注意：未证明已存在可利用 XSS；应定性为高影响渲染防御缺口。

### 5.11 CORS 同源判断使用子串匹配（已修复）

- 严重性：当前 Low（JWT 经 `Authorization` 头传递，非 cookie，跨域页面拿不到 token）；若按 §7 把 JWT 迁移到 cookie，则升为 High
- 优先级：P2（本次已修复）
- 证据：
  - 修复前 `corsMiddleware` 用 `strings.Contains(origin, c.Request.Host)` 判同源：`server/internal/handler/router.go`
  - 配合 `Access-Control-Allow-Credentials: true`
- 风险：`https://<host>.evil.com` 之类 origin 会被子串命中、判为同源并放行。当前因凭据走 `Authorization` 头、跨域页面无法读取受保护响应，实际可利用性低；但与 §7「JWT 迁移到 HttpOnly cookie」建议直接冲突——一旦迁移即变为可利用的凭据/数据泄露面。
- 修复：改为解析 origin 后用 `strings.EqualFold(u.Host, c.Request.Host)` 精确比较，与 WS `CheckOrigin`（`server/internal/middleware/origin.go`）保持一致。
- 关联：与 Nezha CVE-2026-53519 同属「子串匹配替代严格校验」错误类别（见 §3.5）。

---

## 6. 运行态数据与 Secret 管理发现

### 6.1 运行态 DB/WAL 文件被 Git 跟踪

`git ls-files` 显示以下文件已被仓库跟踪：

- `server/data/tolato.db`
- `server/data/tolato.db-shm`
- `server/data/tolato.db-wal`

**更正（2026-06-17，经维护者确认）**：被跟踪的是**本地开发用的假数据库**，其中不含真实 secret。因此本条的实际性质是**仓库卫生问题**（运行态文件不应被跟踪，否则每次本地运行都产生 diff 噪音），而非凭据泄露；初稿中「立即轮换凭据」对本仓库不适用。

处置：

1. 已停止跟踪运行态 DB/WAL/SHM（commit `cb9c84f`，`git rm --cached`，本地文件保留）；`.gitignore` 早已含 `/server/data/`，对已跟踪文件无效是本次需手动移除的原因。
2. 仅当未来确有真实运行态 DB 被误提交时，才需要 secret 扫描、历史清理与凭据轮换。
3. 提交报告/代码时只 stage 目标文件，避免误提交运行态数据变化。

### 6.2 Settings 明文存储

- `Setting.Value` 是 JSON 字符串：`server/internal/model/models.go:124-130`
- 保存路径未见加密：`server/internal/store/setting.go:28-63`
- LLM/Jina key GET 有 masking，但 PUT 保存路径仍是 settings：`server/internal/handler/setting.go:49-96`、`server/internal/handler/setting.go:284-333`

`EncryptKey` 存在于 config，但本次未发现其对 settings secret 的实际加密落地。不能写成“默认 encrypt key 可解密所有 secret”；应写成“未见关键 settings 的 at-rest encryption 使用”。

**更正（2026-06-17，经维护者确认）**：config 中的 `JWTSecret`、`EncryptKey` 默认值是**开源仓库的占位符**，并非泄露的真实密钥。真实风险点不在「仓库泄露了真 secret」，而在「生产部署是否强制覆盖这些占位符」。建议的加固：服务启动时若检测到仍为默认占位符，应拒绝启动或打出显著告警，避免生产环境忘记替换。

---

## 7. 整改路线图

### 7.1 第 0 阶段：立即止血（1–2 天）

1. 移除仓库跟踪的运行 DB/WAL/SHM 与本地生产 config；更新 `.gitignore`；运行 secret scanning。
2. 轮换可能暴露的 JWT secret、agent secret、registration token、API keys、LLM/Jina/notify tokens。
3. `/ws/terminal` 加 `SetReadLimit`、heartbeat、idle timeout、session cap。
4. `file_op` 加并发上限、写入大小上限、list 条目上限；默认禁用写/删或要求二次确认。
5. External/MCP/Chat command 设置最大 timeout 与输出大小上限。
6. `update_agent` 加用户确认与审计。
7. 增加最小 CSP、安全响应头、URL scheme allowlist。

### 7.2 第 1 阶段：控制面加固（1–2 周）

1. agent secret 哈希存储、轮换；registration token 短 TTL/一次性/使用次数限制。
2. settings secret envelope encryption；旧明文迁移。
3. agent update signed manifest + checksum/signature 验证。
4. API key scopes + node allowlist + TTL。
5. 安全事件审计：terminal、file_op、update、settings、API key、agent register/reconnect/replace。
6. LLM tool policy：高危 tool 确认绑定 `tool_call_id + args hash + TTL`；tool output/web content 标记为不可信。

### 7.3 第 2 阶段：架构演进（2–6 周）

1. Terminal/file_op capability token：绑定 user/JWT、node、capability、expiry、nonce。
2. agent challenge-response 或 mTLS；新注册节点默认隔离，确认可信后开放高危能力。
3. 全局 resource budget：per-user、per-key、per-node、per-agent、global。
4. 如需多用户/企业化：新增 users/roles/tenants/ownership/policy engine。
5. session recording、JIT access、break-glass、SIEM/告警集成。

---

## 8. 最低安全基线

### WebSSH / terminal / file_op

- WebSocket auth 前后均有 read limit。
- terminal 有 heartbeat、idle timeout、最大会话时长。
- 每 node/session/global 有 session 上限。
- file_op 有并发、payload、write size、list entries、read bytes 限制。
- write/delete/mkdir 默认需要确认或可配置禁用。
- PTY output 有带宽/队列上限；浏览器端链接经过 sanitizer。
- terminal/file_op 全量元数据审计。

### Agent 身份

- registration token 短 TTL，默认一次性或有限次数。
- agent secret 哈希存储，不进 URL query 或日志。
- agent reconnect / replacement 进入审计与告警。
- 支持 secret rotation。

### 更新链

- artifact 必须校验 digest 和签名。
- agent 内置信任根 public key。
- update 需要审计、确认、冷却时间。
- 更新失败可回滚。

### Secrets / Settings / DB

- 运行 DB 与本地配置不进入 Git。
- settings 中 API key/token 加密存储。
- GET API 只返回 masked secret。
- 支持 secret rotation 与明文迁移。
- CI 运行 secret scan。

### API key / MCP

- API key 具备 scopes、TTL、node allowlist。
- readonly 不能通过 MCP 间接执行写操作。
- MCP body/batch/tool call 有资源限制。
- 所有 MCP 高危 tool 进入审计。

### LLM tool

- tool result/web content/node metadata 均视为不可信。
- 高危 tool 需要人类确认。
- 确认绑定 `tool_call_id + args hash + TTL`。
- 限制每轮 tool 数量、并发、timeout。
- prompt-injection 测试进入回归集。

### 前端

- CSP 默认启用。
- Markdown 禁 HTML 或严格 sanitize。
- `v-html` 仅用于可信 sanitizer 输出。
- URL 只允许 `http:` / `https:`。
- JWT 不长期存 localStorage，至少规划迁移到 HttpOnly cookie/内存 token。

### 审计告警

- 覆盖 command、terminal、file_op、update、settings、API key、agent register/reconnect/replace。
- 审计输出截断和脱敏。
- 高危事件通知。
- 审计记录包含 actor、source、resource、result、request id。

---

## 9. 最终建议用语与避免过度断言

应避免：

- “已确认 XSS” → 改为“存在高影响渲染面与防御纵深不足，未完成动态 PoC”。
- “任意用户越权访问任意节点” → 当前应为“单管理员全局控制面；若未来多用户则为 Critical broken access control”。
- “WebSocket 完全没有 Origin 检查” → 实际已有 CheckOrigin；重点是 wildcard/部署配置、认证后授权与资源限制。
- “ReleaseProxy 可直接导致 RCE” → 改为“更新链缺少签名/checksum/TUF 类完整性校验；若上游/代理/发布资产被污染，影响高”。
- “DoS 已验证” → 改为“代码层缺少资源边界，具备 DoS 风险；未压测”。
- “settings 已泄漏真实 secret” → 改为“代码路径显示 settings 以 JSON 字符串存储，GET 有 masking，未发现有效加密落地；真实 secret 未读取”。

---

## 10. 结论

Tolato 的 WebSSH 不是传统 OpenSSH，而是：浏览器 JWT 会话 → server `/ws/terminal` → agent 本地 PTY/file API 的控制通道。该设计使 Web 登录态成为所有在线节点的交互式主机控制凭据。

在当前单管理员模式下，这是一项强管理功能，但必须配套强约束：短期 capability token、节点/能力授权、file sandbox、资源限制、会话审计、signed update、agent 身份轮换、secret 加密和安全默认值。

若 Tolato 未来进入多用户/企业团队场景，当前模型必须先引入用户/角色/租户/节点 ownership/policy engine，否则会形成系统性 Critical 访问控制缺陷。
