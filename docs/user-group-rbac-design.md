# Tolato 用户体系与 Group 权限设计

> 状态：**已全部实现**（P1–P4）。撰写日期：2026-08-04，基于 commit `f126361`。
> 实现与设计的差异见文末「§9 实现记录」。
> 前置阅读：[multi-tenancy-assessment.md](multi-tenancy-assessment.md)（2026-06 的三档方案评估）。
> 本文对应当时的「方案 B+」——多用户 + 组权限，但**不做**互不信任的 SaaS 租户隔离。

## 1. 目标与非目标

### 目标

- 多用户登录，取代 config 里的单一 `admin/admin`
- **用户组（UserGroup）** 与 **节点组（NodeGroup）** 两侧分组
- 权限授予（Grant）：把「用户或用户组」授权到「节点、节点组或全部节点」，带权限级别
- 所有访问面统一走同一套鉴权：Web UI / WebSSH 终端 / AI 对话工具调用 / 外部 API / MCP
- 平滑迁移：升级后现有部署行为不变（原 admin 自动成为管理员，看得到一切）

### 非目标

- 不做 SaaS 多租户（互不信任的组织共享一套实例）——那是另一个量级的隔离
- 不做资源级细粒度到「某 key 只能在 nodeA 上跑白名单命令」——命令白名单仍是全局安全设置
- 不拆分 Settings 为 per-user（LLM key、通知渠道等保持全局，仅管理员可改）

## 2. 概念模型

```
User ──┬── 属于 N 个 ──> UserGroup
       │
       └─(或直接)─┐
                  ├──> Grant(level) ──┬──> Node
UserGroup ────────┘                   ├──> NodeGroup <── 包含 N 个 ── Node
                                      └──> all（全部节点）
```

一条 Grant = **主体**（user 或 user_group）+ **客体**（node、node_group 或 all）+ **级别**。

用户对某节点的**有效权限 = 所有命中 Grant 的最高级别**（主体侧取本人 + 所在用户组；客体侧取该节点 + 节点所在组 + all）。

### 权限级别（对节点的操作能力，递进包含）

| 级别 | 能力 |
|------|------|
| `viewer` | 节点在列表可见；查看状态、指标、详情、Extra 元数据；查看该节点的审计日志 |
| `operator` | + 执行命令（AI 对话、WebSSH 终端、file_op）；触发的命令进入审计 |
| `manager` | + 编辑节点（alias、Extra）、触发 agent 升级、删除节点 —— **仅限 Web UI**；AI 对话（非 admin）、CLI、API Key 一律没有节点属性修改能力（见 §4 点5、§5.4） |

对节点**没有任何 Grant = 不可见**（列表里不出现，直接访问 404 而非 403，避免泄露存在性）。

### 全局角色（与节点级别正交）

| 角色 | 能力 |
|------|------|
| `admin` | 一切：用户/组/Grant 管理、全局 Settings、注册 token、所有节点的 manager 权限（隐式，无需 Grant）、全部审计日志 |
| `member` | 只能做被 Grant 授予的事；可管理自己的账号（改密码）、自己的对话、自己创建的 API Key |

只有两个全局角色，刻意不搞多级——节点侧的差异全部用 Grant 表达。

## 3. 数据模型

新增六张表，现有表加归属字段。全部走 GORM auto-migration（Postgres）。

```go
type User struct {
    ID           string    // uuid
    Username     string    // uniqueIndex
    PasswordHash string    // bcrypt，json:"-"；OIDC 用户为空
    DisplayName  string
    Role         string    // admin | member
    Status       string    // active | disabled
    AuthSource   string    // local | oidc
    OIDCSubject  *string   // uniqueIndex；OIDC 的 sub claim
    Email        string    // OIDC 建号时回填；admin_emails 白名单匹配用
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

type UserGroup struct {
    ID          string
    Name        string    // uniqueIndex
    Description string
    CreatedAt   time.Time
}

type UserGroupMember struct {
    UserGroupID string // 联合主键
    UserID      string // 联合主键
}

type NodeGroup struct {
    ID          string
    Name        string    // uniqueIndex
    Description string
    CreatedAt   time.Time
}

type NodeGroupMember struct {
    NodeGroupID string // 联合主键
    NodeID      string // 联合主键
}

type Grant struct {
    ID          string
    SubjectType string // user | user_group
    SubjectID   string
    ObjectType  string // node | node_group | all
    ObjectID    string // ObjectType=all 时为空串
    Level       string // viewer | operator | manager
    CreatedBy   string // 授予人 user_id，审计用
    CreatedAt   time.Time
    // uniqueIndex(subject_type, subject_id, object_type, object_id)
    // 同一主客体只存一条，重复授予=更新级别
}
```

节点与用户组都是**多对多**：一台机器可以同时在 `prod` 和 `hk` 两个组，一个用户可以同时在 `ops` 和 `dev`。

### 现有表的改动

| 表 | 改动 |
|----|------|
| `conversations` | + `user_id`（对话私有，只有本人可见；admin 也看不到别人的对话） |
| `audit_logs` | + `user_id`（保留现有 `actor` 字符串做展示兜底）；查询按节点可见性过滤 |
| `api_keys` | + `owner_user_id`；`permission` 从三档收敛为 `readonly` / `writable`（见 §5.4） |
| `registration_tokens` | + `node_group_id`（可空）：用该 token 注册的节点自动加入此组 —— 这是「机器进组」的主通道 |
| `nodes` | 不加 owner 字段。节点没有属主，归属关系全部由 NodeGroupMember + Grant 表达 |
| `settings` | 不动，仍全局；**读写全部 admin only**（member 连 GET 都不给——LLM key 掩码、通知 webhook 地址等对 member 也是敏感信息） |

### 有效权限的一次查询

```sql
SELECT g.level FROM grants g
WHERE (
      (g.subject_type = 'user'       AND g.subject_id = $user_id)
   OR (g.subject_type = 'user_group' AND g.subject_id IN
        (SELECT user_group_id FROM user_group_members WHERE user_id = $user_id))
) AND (
      g.object_type = 'all'
   OR (g.object_type = 'node'       AND g.object_id = $node_id)
   OR (g.object_type = 'node_group' AND g.object_id IN
        (SELECT node_group_id FROM node_group_members WHERE node_id = $node_id))
);
-- Go 侧取 max(level)；admin 角色直接短路返回 manager
```

「我可见的节点集合」是同构的反向查询（返回 node_id 列表），供 `ListNodes` 和仪表盘用。自托管规模（几十节点、个位数用户）下每请求直查即可，**不需要缓存**；将来慢了再在进程内缓存 user→nodeset，Grant/成员变更时失效。

## 4. 鉴权层设计

新增 `internal/authz` 包，唯一入口，两个函数：

```go
// 该用户对该节点的有效级别；admin 恒为 manager；无权限返回 ("", false)
func NodeLevel(userID string, isAdmin bool, nodeID string) (Level, bool)

// 该用户可见的节点 ID 集合；admin 返回全部
func VisibleNodeIDs(userID string, isAdmin bool) ([]string, error)
```

JWT Claims 从 `{username}` 扩展为 `{user_id, username, role}`。`JWTAuth()` 注入三者到 gin context。**所有**权限判断都收敛到 authz 包，handler / WS / MCP 不各自写 SQL。

### 强制点清单（缺一处就是洞）

| # | 入口 | 检查 |
|---|------|------|
| 1 | `GET /api/nodes` | 按 `VisibleNodeIDs` 过滤 |
| 2 | `GET/PUT/DELETE /api/nodes/:id`、`/update` | GET 要 viewer；PUT/update/DELETE 要 manager；无权限一律 404 |
| 3 | `GET /api/nodes/:id/commands`、`/api/audit-logs` | 按节点可见性过滤；audit-logs 全局视图仅 admin |
| 4 | `WS /ws/terminal`（WebSSH） | 建连时对目标 node 要 **operator**；升级 WS 前校验 |
| 5 | `WS /ws/chat` → LLM 上下文与工具 | **源头收口**：注入给模型的节点列表 / `list_nodes` 工具结果在代码层固定为该用户的可见集合，模型从头到尾接触不到无权节点；`edit_node` 工具**仅 admin 的会话注入**，member 的工具集里没有这个工具。工具分发层仍保留执行前校验（operator）作为兜底——防模型凭空拼 node ID |
| 6 | 对话里选择 `default_node_id` | 校验 viewer，防止借对话探测节点存在性 |
| 7 | `/api/v1/*`（API Key，CLI 的后端） | 见 §5.4，key 继承 owner 的节点范围；v1 **不提供**节点属性修改端点 |
| 8 | Settings 全部页与接口（LLM/安全/Agent/Chat/WebFetch/通知/OIDC，含读和写）、用户/组/Grant/注册 token | admin only。**唯一例外是 API Keys**：member 可访问，但 list/delete 一律 `WHERE owner_user_id = 本人`，只能建/看/删自己的 key |

第 5 点的原则是「泄漏在源头堵死」：可见性靠预过滤的上下文保证，能力靠按角色裁剪的工具集保证（member 会话根本没有 `edit_node`），执行前校验只是双保险，不是第一道防线。

## 5. 各子系统的具体方案

### 5.1 登录与用户引导

- `config.yaml` 的 `auth.username/password` 保留，语义变为**首次启动引导**：DB 无任何用户时，用它创建第一个 `admin`；此后配置项不再参与登录（打日志提示可删除）。
- 密码登录改查 `users` 表 + bcrypt 比对。保留现有登录失败限速逻辑（若有）。
- 用户管理 API（admin）：增删用户、重置密码、启停用、改角色。用户自助：改自己密码。
- 不做注册页/邀请流/邮箱验证——自托管场景 admin 手工建号足够。

**OIDC 登录**（可选启用，与密码登录并存）：

- **配置存 `settings` 表、admin 在界面上配**，与 LLM / 通知渠道设置同一形态，不进 config.yaml、不用重启：
  - 新增 Settings 页签「SSO / OIDC」+ `GET/PUT /api/settings/oidc`（admin only）
  - 字段：启用开关、`issuer`、`client_id`、`client_secret`、`scopes`（默认 `openid profile email`）、`group_claim`、组映射表、`admin_emails`
  - `client_secret` 写后不回显（GET 返回掩码，同现有 LLM key 的处理方式）
  - 页面上提供「验证」按钮（`POST /api/settings/oidc/verify`：拉 discovery 文档、校验 client 配置），以及**只读展示 callback URL** 供 admin 复制到 IdP 后台
  - 保存后热生效：OIDC provider/verifier 按 settings 惰性初始化，配置变更即重建，登录页的「SSO 登录」按钮按启用开关显隐
- 单 Provider。协议用标准 Authorization Code Flow，discovery 走 issuer 的 `/.well-known/openid-configuration`，库用 `coreos/go-oidc`。
- 端点：`GET /api/auth/oidc/login`（302 到 IdP）+ `GET /api/auth/oidc/callback`（换 token、验签、发本站 JWT）。前端登录页在 OIDC 启用时显示「SSO 登录」按钮。
- **用户匹配与自动建号**：`users` 表加 `auth_source`（`local` | `oidc`）和 `oidc_subject`（唯一索引）。按 `sub` 匹配；首登自动建号，默认角色 `member`。可配 `admin_emails` 白名单：命中者建号即 admin。
- **账号采纳**：首登时若存在同「已验证邮箱」的本地账号，直接接管该账号（保留其 ID、对话、API Key），并清空本地密码——已有管理员切到 SSO 不会变成一个空的新账号。未验证的邮箱不作为归属证据。
- **组映射：暂不实现**。它依赖尚未落地的用户组（P2），且简单接入已能满足需求。将来做 P2 时再补 `group_claim` + 映射表。
- OIDC 用户不可改密码（无本地密码），管理员也不能给其设置密码；被 IdP 停用的用户由 admin 手工 disable 即时生效（JWTAuth 每请求重读状态）。

### 5.2 节点分组

- 组管理 API（admin）：`/api/node-groups` CRUD + 成员增删。
- **注册 token 绑组**：创建注册 token 时可选 node_group，新机器上线即入组、即被相应 Grant 覆盖——运维上这是主要入口，装机脚本不用改。
- 已有节点由 admin 在节点详情页手工调整分组。
- Node 的 `Extra` JSONMap 与分组并存：Extra 管元数据（厂商、到期日），Group 管权限域，不混用。

### 5.3 用户组与 Grant 管理

- `/api/user-groups` CRUD + 成员管理（admin）。
- `/api/grants` CRUD（admin）。除了「权限」页的授权总表，还提供**两个解析后的视角**（授权表说的是「有哪些规则」，这两个回答「规则加起来等于什么」）：
  - `GET /api/users/:id/access` —— 用户页的「查看可访问范围」：这个人能碰哪些机器、什么级别。入职/离职审计用。
  - `GET /api/nodes/:id/access` —— 节点详情页的「谁能访问」：出事时谁碰得到这台机器。会区分「凭 admin 角色」和「凭授权」。
- 支持直接 user→node 的个别授权（表结构天然支持），但 UI 引导以「用户组→节点组」为主路径。

### 5.4 API Key 与 CLI（取代 MCP）

**API Key 简化为两档**（现有 readonly/standard/admin 三档迁移：readonly→readonly，standard/admin→writable）：

| key permission | 节点范围 | 能做什么 |
|----------------|---------|----------|
| `readonly` | owner 可见节点 | 查列表、看详情、看指标 |
| `writable` | owner 可见节点 ∩ owner 有 operator 的节点 | + 执行命令、file_op |

- Key 绑 `owner_user_id`，能力永远是 owner 权限与 key 档位的**交集**；owner 被禁用/删除 → key 立即失效（校验时联查 owner status）。
- **任何 key 都不能修改节点属性**：`/api/v1` 不提供节点编辑端点（现状本来就没有，保持），节点属性修改只存在于 Web UI 会话（manager/admin）。

**对外集成从 MCP 改为 CLI + Skill**：

- 移除 `/mcp` 端点及 `internal/mcp` 包；`/api/v1` REST 保留并成为 CLI 的唯一后端。
- 新增 `tolato` CLI（仓库 `cli/` 目录，Go 单二进制，随 release 分发）：
  - 配置：`TOLATO_URL` + `TOLATO_API_KEY` 环境变量，或 `~/.config/tolato/config.yaml`
  - `tolato nodes list` / `tolato nodes get <id-or-alias>` —— readonly 即可
  - `tolato exec <node> -- <command>`、`tolato file cat|put ...` —— 需 writable
  - **不提供** `nodes edit` 之类的子命令；CLI 的能力面 = v1 端点面，天然不含属性修改
- **CLI 的全部写操作强制入审计**：审计写在服务端 v1 handler 里（`execute` 现状已写 AuditLog，新增的 file_op 端点同样写），客户端无法绕过。改造点：
  - AuditLog 补 `user_id`（= key 的 owner），追责到人而不只是到 key
  - `source` 区分 `cli`（User-Agent 为 `tolato-cli/*` 时）与 `api`（其他 v1 客户端），审计页可按来源筛
  - readonly 操作（list/get）不入审计，避免噪音；写操作（execute、file_op）全量记录命令、退出码、stdout/stderr、耗时
- 随仓库发布一个 **Claude Skill**（`skills/tolato/SKILL.md`）：描述 CLI 的安装、鉴权配置和各子命令的使用方式，用户装进 Claude Code 后，AI 通过跑 CLI 来操作节点。权限完全由 API Key 收口，Skill 本身零权限逻辑。

### 5.5 对话与审计

- 对话严格私有（`WHERE user_id = ?`），包括 admin——admin 管机器，不看人的聊天记录；需要追责时看审计日志，那里有完整命令与输出。
- 审计日志写入时带 `user_id`（终端/对话）或 `api_key_id`→owner（外部 API）；读取按节点可见性过滤，admin 全量。

### 5.6 前端

- Pinia store 增加 `user`（id/username/role）；登录响应带回。
- Settings 对 `member` 只显示 **API Keys** 一页（仅本人的 key）；LLM、安全、Agent、Chat、WebFetch、通知、OIDC 各页及注册 token 页一律隐藏（后端接口同步 admin only，前端隐藏只是体验，不是防线）。新增：用户管理、用户组、节点组、授权管理四个 admin 页面。
- 节点列表天然按接口返回过滤，前端无需权限逻辑；操作按钮按接口返回的 `my_level` 字段显隐（`GET /nodes` 每项带上当前用户的有效级别）。

## 6. 迁移与兼容

一次性迁移逻辑（启动时自动执行，幂等）：

1. auto-migration 建新表、加新列。
2. `users` 为空 → 用 config auth 创建 admin（bcrypt 加密）。
3. 存量 `conversations.user_id` 为空 → 归到该 admin。
4. 存量 `api_keys.owner_user_id` 为空 → 归到该 admin。
5. 不自动建任何 NodeGroup / Grant——admin 隐式全权，升级后单人使用体验零变化；分组是增量启用的功能。

旧 JWT（无 user_id）在升级后自然失效（校验 claims 缺字段即 401），用户重登一次即可，不做兼容分支。

## 7. 实施分期

| 期 | 内容 | 量级 |
|----|------|------|
| **P1 账号体系** ✅ | users 表、bcrypt 登录、JWT 带 user_id/role、引导迁移、用户管理 API+页面、对话归属、审计带 user_id、API Key 两档+绑 owner | 已完成 |
| **P2 分组与授权** ✅ | 五张新表、authz 包、REST 层强制点 1–3、6、8、注册 token 绑组、管理 API+页面 | 已完成 |
| **P3 执行面收口** ✅ | 终端 WS（点4）、chat 上下文预过滤 + 按角色裁剪工具集（点5）、API Key 两档制 + 绑 owner（点7） | 已完成 |
| **P3.5 CLI + Skill** ✅ | `tolato` CLI、下线 `/mcp` 与 `internal/mcp`、`skills/tolato/SKILL.md`、v1 审计补 user_id/source | 已完成 |
| **P3.6 OIDC** ✅ | go-oidc 接入、Settings 页配置 + verify + 热生效、auth_source/oidc_subject、自动建号、账号采纳（组映射留给 P2） | 已完成 |
| **P4 打磨** ✅ | `my_level` 字段与按钮显隐、审计过滤细化、文档 | 已完成 |

P1 独立可发布（等价于评估文档的方案 B）；P2/P3 必须一起发（只发 P2 会出现「列表看不见但终端连得上」的假隔离）。总量约 2–2.5 周，与当年评估的方案 C 估算一致，但砍掉了 Settings 拆分和租户隔离两块最重的部分。

## 8. 关键取舍记录

1. **Node 不设 owner**——机器是共享资源，归属由组表达；避免「owner 离职机器变孤儿」。
2. **Grant 主体允许单用户**——表结构统一，成本为零；但 UI 主推组授权，避免授权表碎片化。
3. **无权限返回 404 而非 403**——不泄露节点存在性。
4. **admin 不可见他人对话**——聊天是人机私聊，追责走审计日志；这条如果产品上不接受，改一行查询即可，但默认从严。
5. **level 存字符串、Go 里映射序数比较**——可读性优先，三档不会膨胀到需要位掩码。
6. **每请求直查不缓存**——规模不需要；authz 单入口保证将来加缓存只改一处。
7. **泄漏在源头堵死，不靠运行时拦截**——AI 对话注入的节点列表和工具集在代码层按用户裁剪（member 无 `edit_node`），执行前校验仅作双保险。
8. **节点属性修改只留一个入口**——Web UI 会话（manager 级别或 admin）。CLI / API Key / member 的 AI 对话一律无此能力，攻击面最小化。
9. **MCP 换成 CLI + Skill**——CLI 的能力面被 `/api/v1` 端点面硬约束，比 MCP 工具层自行实现权限检查更不容易出错；Skill 只是使用说明，零权限逻辑。
10. **API Key 两档而非三档**——readonly / writable 覆盖全部真实场景；「admin 级 key」这种大杀器不该存在。


## 9. 实现记录

实现过程中与设计稿的差异，均为收紧而非放宽：

1. **少一张表**。设计稿写「六张新表」，实际五张：`user_groups`、`user_group_members`、`node_groups`、`node_group_members`、`grants`。原先把 Grant 的唯一索引单独算了一张。
2. **OIDC 组映射未做**。它依赖用户组，而 OIDC 先于 P2 落地；产品上简单接入已够用，后续需要时再补 `group_claim` + 映射表即可（表已就绪）。
3. **AI 对话的工具集按角色裁剪，而非调用时拒绝**。`edit_node_info` 和 `update_agent` 对 member **根本不注入**，模型看不到就无从调用；工具分发层仍保留 admin + manager 双重校验作为兜底。
4. **API Key 的能力是「档位 ∩ owner 权限」**。writable key 也只能在 owner 拥有 operator 的节点上执行；owner 停用则 key 立即失效。
5. **删除的级联比设计稿更彻底**。删用户 → 同时清其组成员身份与 grants；删节点 / 节点组 → 同时清 grants，节点组还会解绑注册 token。避免 id 复用时权限「诈尸」。
6. **列表分页在 SQL 层按可见集过滤**（`ListNodesScoped` / `ListAuditLogsScoped`），而不是取出后再筛——否则只有两台机器权限的用户会翻到大量空页。
7. **无权限一律 404**，包括审计日志按 node_id 过滤时，避免用结果数量探测节点存在性。
8. **「两个视角」按设计做了**，但形式是解析后的只读弹窗（`/users/:id/access`、`/nodes/:id/access`），而不是在各自详情页里内嵌一份可编辑的授权表——授权的增删统一留在「权限」页一处，避免同一份数据出现两个可写入口。
