# NodeProbe — 链路监控模块设计（tolato 集成方案）

## 1. 定位

NodeProbe 是 tolato 的一个**内置子模块**，不是独立项目。它复用 tolato 已有的 Node Agent 通道、节点列表、认证体系，只需要：

- Agent 端：新增探针采集能力（ping / tcp / bandwidth）
- Server 端：新增指标存储 + 告警引擎
- 前端：新增一个「链路监控」页面（含拓扑图 + 详情 + 告警）

---

## 2. 与 tolato 的整合点

| 已有能力 | NodeProbe 如何复用 |
|---------|------------------|
| Node Agent 进程 | 探针逻辑嵌入同一个 agent 二进制，不需要单独部署 |
| Agent ↔ Server WebSocket | 探针配置由 Server 通过 WS 下发；指标通过 HTTP POST 上报（与 WS 解耦，避免阻塞命令通道） |
| 节点注册 + 心跳 | 探针的「节点在线/离线」直接复用心跳状态，不重复检测 |
| 节点列表 + 元数据 | 在节点上扩展 `role`（entry/relay/landing）和 `upstream_id` 字段来描述拓扑关系 |
| auth_token 认证 | 探针上报复用同一个 agent token |
| 数据库（PostgreSQL） | 新增 probe 相关表，与现有表共用同一个数据库 |
| 前端侧边栏 | 新增「链路监控」导航入口 |

---

## 3. 网络拓扑模型 — 交互式画布

### 3.1 核心交互：画布 + 拉线

拓扑关系**不再通过配置字段定义**，而是通过**可视化画布**直接操作：

- 画布上展示所有已注册的在线节点（从 Nodes 列表自动同步）
- 用户可以**自由拖拽**节点到任意位置（位置持久化到数据库）
- 用户从节点 A **拖出一条线**连接到节点 B → 自动创建 `A → B` 的监控链路
- 删除连线 → 删除对应的监控链路
- 连线上实时显示链路指标（延迟、丢包率）

### 3.2 节点

节点自动从已注册的 Node Agent 列表获取，不需要手动添加。节点卡片显示：

- 状态灯（绿/黄/红）
- 节点名称
- 角色标签（entry / relay / landing，可在画布上右键编辑）

**节点位置**持久化到数据库，每个节点存储 `canvas_x` 和 `canvas_y`。

### 3.3 链路（连线）

链路通过画布上的连线定义：

- 从源节点拖出连线 → 松开到目标节点 → 创建链路
- 链路 ID 格式：`{source_id}->{target_id}`
- 创建链路后，Server 自动通过 WebSocket 下发探针配置给源节点的 Agent
- 删除连线时右键菜单或选中后按 Delete

**连线样式：**
- 贝塞尔曲线 + 箭头，方向从源到目标
- 颜色反映链路状态：绿色=正常、橙色=警告、红色=告警、灰色=无数据
- 连线中间显示指标标签（`32ms | 0%`）
- 鼠标悬浮连线 → tooltip 显示完整四项指标
- 点击连线 → 跳转链路详情页

### 3.4 画布交互

| 操作 | 行为 |
|------|------|
| 拖拽节点 | 移动节点位置（自动保存） |
| 从节点边缘拖出 | 创建新连线（松开到目标节点上完成） |
| 鼠标滚轮 | 缩放画布 |
| 按住空白处拖拽 | 平移画布 |
| 右键节点 | 编辑角色、查看详情 |
| 右键连线 | 查看详情、删除连线 |
| 点击连线标签 | 跳转链路详情页 |

### 3.5 数据模型变更

节点表扩展字段：

```
canvas_x:  REAL    — 画布上的 X 坐标
canvas_y:  REAL    — 画布上的 Y 坐标
role:      TEXT    — entry / relay / landing（可选标签）
```

链路表（probe_links）完全由画布上的连线决定，不再需要 `upstream_id` 字段。

---

## 4. Agent 端：探针模块

### 4.1 集成方式

探针作为 Node Agent 的一个**可选子模块**，由 Server 下发配置来激活：

```
Agent 启动
  → 连接 Server，完成注册
  → Server 根据该节点的 role 和下游节点列表，下发探针配置
  → Agent 收到配置后启动探针调度器
  → 节点拓扑变更时，Server 推送新配置，Agent 热更新
```

这样 Agent 端**不需要本地配置文件来指定探测目标**，一切由 Server 中心管控。

### 4.2 Server 下发的探针配置

通过现有 WebSocket 通道下发：

```json
{
  "type": "probe_config",
  "payload": {
    "enabled": true,
    "report_url": "http://<server>/api/v1/probe/report",
    "targets": [
      {
        "id": "relay-a",
        "name": "中转A-东京",
        "host": "10.0.1.1",
        "ping_count": 10,
        "tcp_port": 443,
        "bandwidth_url": "http://10.0.1.1:9090/testfile"
      }
    ]
  }
}
```

触发时机：
- Agent 首次注册后
- 用户在前端修改节点拓扑关系后
- 新增/删除下游节点后

### 4.3 采集指标

| 指标 | 方式 | 频率 |
|------|------|------|
| 延迟（latency_min/avg/max） | ICMP Ping（N 个包） | 每 30s |
| 丢包率（packet_loss） | 同一次 Ping 任务统计 | 每 30s |
| TCP 握手耗时（tcp_connect_time） | 对目标端口发起 TCP 连接计时 | 每 30s |
| 带宽（bandwidth_mbps） | HTTP 下载测速 | 每 5min |

### 4.4 上报格式

通过 HTTP POST 上报（不走 WebSocket，避免大量指标数据阻塞命令通道）：

```
POST /api/v1/probe/report
Authorization: Bearer <agent_token>
```

```json
{
  "node_id": "entry-1",
  "timestamp": "2026-03-31T15:30:00+08:00",
  "metrics": [
    {
      "target_id": "relay-a",
      "latency_min": 12.3,
      "latency_avg": 15.8,
      "latency_max": 23.1,
      "packet_loss": 0.0,
      "tcp_connect_time": 18.5,
      "bandwidth_mbps": null
    }
  ]
}
```

### 4.5 带宽测试文件服务

被测节点（中转/落地）上需要跑一个简单 HTTP 文件服务提供测试文件下载。作为 Agent 的子命令：

```bash
./tolato-agent serve-testfile --port 9090 --size 10
```

生成一个 10MB 的内存文件，通过 HTTP 提供下载。

---

## 5. Server 端

### 5.1 新增模块

```
internal/
  └── probe/
      ├── store.go       # 数据库操作（指标、告警的 CRUD）
      ├── alert.go       # 告警引擎（阈值检测 + 恢复判断）
      ├── telegram.go    # Telegram Bot 通知
      ├── api.go         # HTTP API handlers
      └── types.go       # 探针相关数据结构
```

### 5.2 数据库表（与现有表共用同一个 PostgreSQL）

**probe_nodes**（探针视角的节点信息，补充 role 等字段）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT PK | 节点 ID（与 nodes 表对应） |
| name | TEXT | 节点名称 |
| role | TEXT | entry / relay / landing |
| upstream_id | TEXT | 上游节点 ID |
| last_seen | TIMESTAMPTZ | 最后上报时间 |

**probe_links**（链路定义）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT PK | 链路 ID（source_id->target_id） |
| source_id | TEXT FK | 源节点 |
| target_id | TEXT FK | 目标节点 |

**probe_metrics**（监控数据）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGSERIAL PK | 自增 |
| link_id | TEXT FK | 链路 ID |
| timestamp | TIMESTAMPTZ | 采集时间 |
| latency_min | REAL | 最小延迟 ms |
| latency_avg | REAL | 平均延迟 ms |
| latency_max | REAL | 最大延迟 ms |
| packet_loss | REAL | 丢包率 % |
| tcp_connect_time | REAL | TCP 握手耗时 ms |
| bandwidth_mbps | REAL | 带宽 Mbps（可为 NULL） |

索引：`(link_id, timestamp DESC)`

**probe_alerts**（告警记录）

| 字段 | 类型 | 说明 |
|------|------|------|
| id | BIGSERIAL PK | 自增 |
| link_id | TEXT FK | 链路 ID |
| type | TEXT | latency / packet_loss / tcp / bandwidth / offline |
| message | TEXT | 告警内容 |
| triggered_at | TIMESTAMPTZ | 触发时间 |
| resolved_at | TIMESTAMPTZ | 恢复时间（NULL=未恢复） |

索引：`(link_id, triggered_at DESC)`

### 5.3 告警引擎

**默认阈值（server config 可调）：**

| 指标 | 告警条件 | 恢复条件 |
|------|---------|---------|
| 延迟 | avg > 200ms | 连续 3 次 < 150ms |
| 丢包率 | > 5% | 连续 3 次 < 1% |
| TCP 耗时 | > 500ms | 连续 3 次 < 300ms |
| 带宽 | < 10Mbps | 连续 2 次 > 20Mbps |
| 节点离线 | 超过 3 分钟未上报 | 恢复上报 |

**告警流程：**
1. 每次收到 metric 上报 → 检查是否超阈值
2. 超阈值 + 该链路无同类型未恢复告警 → 创建告警 + 发 Telegram
3. 未超阈值 + 该链路有同类型未恢复告警 → 累计恢复计数，达到阈值后标记恢复 + 发 Telegram
4. 离线检测：每 60 秒巡检，发现 last_seen 超时的节点 → 对其所有链路创建 offline 告警

**Telegram 消息格式：**

告警：
```
🔴 告警：链路异常
━━━━━━━━━━━━━
链路：入口-香港 → 中转A-东京
类型：延迟过高
当前值：延迟 356ms（阈值 200ms）
时间：2026-03-31 15:30:00 UTC+8
```

恢复：
```
🟢 恢复：链路恢复正常
━━━━━━━━━━━━━
链路：入口-香港 → 中转A-东京
类型：延迟恢复
当前值：延迟 45ms
持续时间：12 分钟
时间：2026-03-31 15:42:00 UTC+8
```

### 5.4 数据清理

后台定时任务，每小时执行一次，清理超过 N 天（默认 30）的 probe_metrics 和已恢复的 probe_alerts。

### 5.5 API 路由

所有路由挂在现有 Gin router 的 `/api/v1/probe/` 下：

```
POST /api/v1/probe/report                 — Agent 上报指标（Bearer Token 认证）
GET  /api/v1/probe/nodes                  — 获取探针节点列表（含 role）
GET  /api/v1/probe/links                  — 获取所有链路 + 最新指标 + 状态
GET  /api/v1/probe/links/:id/metrics      — 获取某条链路历史指标（?from=&to=）
GET  /api/v1/probe/alerts                 — 获取告警列表（?link_id=&type=&status=）
```

---

## 6. 前端：链路监控页面

### 6.1 导航入口

左侧边栏新增一项：

```
- 对话
- Nodes
- 链路监控  ← 新增
- 审计日志
- 系统设置
```

### 6.2 监控首页 — 拓扑总览

**路由：** `/monitor`

**页面结构：**

```
┌─────────────────────────────────────────────────────────┐
│  链路监控                                                │
├─────────────────────────────────────────────────────────┤
│  [总链路: 12]  [正常: 10]  [警告: 1]  [告警: 1]          │
├─────────────────────────────────────────────────────────┤
│                                                         │
│               画布拓扑图区域                               │
│                                                         │
│   ┌──────┐         ┌──────┐         ┌──────┐           │
│   │🟢    │  32ms   │🟢    │  15ms   │🟢    │           │
│   │入口HK │───0%──→│中转A │───0%──→│落地1 │           │
│   │entry │         │relay │    │    │landing│           │
│   └──────┘    │    └──────┘    │    └──────┘           │
│               │         │      │    ┌──────┐           │
│               │         │  28ms│    │🟢    │           │
│               │         └──1%──┼──→│落地2 │           │
│               │                │    │landing│           │
│               │    ┌──────┐    │    └──────┘           │
│               │    │🔴    │    │    ┌──────┐           │
│               │356m│中转B │  45ms   │🟢    │           │
│               └─5%→│relay │───0%──→│落地X │           │
│                    └──────┘         │landing│           │
│                                     └──────┘           │
│                                                         │
├─────────────────────────────────────────────────────────┤
│  最近告警                                                │
│  时间              链路                    类型    当前值  │
│  03-31 15:30:00   入口HK → 中转B-新加坡   延迟   356ms   │
│  03-31 14:12:00   中转A → 落地2           丢包   8%     │
│  ...                                                    │
└─────────────────────────────────────────────────────────┘
```

**拓扑图设计：**
- 三列布局：入口（左）→ 中转（中）→ 落地（右）
- 节点卡片：状态灯（绿/黄/红）+ 名称 + 角色标签，不显示指标数据
- 连接线：贝塞尔曲线 + 箭头，颜色反映链路状态（绿/黄/红）
- 连线中间小标签：显示 `延迟 | 丢包率`
- 鼠标悬浮标签 → tooltip 浮层显示完整四项指标
- 点击连线标签 → 跳转链路详情页
- 点击节点卡片 → 跳转该节点关联的链路列表
- 支持鼠标滚轮缩放 + 拖拽平移（节点多时不拥挤）
- 落地节点按所属中转分组，上下排列

**底部：** 最近 10 条告警，点击跳转详情。

### 6.3 链路详情页

**路由：** `/monitor/:linkId`

```
┌─────────────────────────────────────────────────────────┐
│  ← 返回    入口-香港 → 中转A-东京                         │
├─────────────────────────────────────────────────────────┤
│  时间范围: [1h] [6h] [24h] [7d]                          │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  延迟趋势 (ms)                  丢包率趋势 (%)            │
│  ┌─────────────────┐           ┌─────────────────┐     │
│  │   折线图          │           │   面积图          │     │
│  │  min/avg/max     │           │                  │     │
│  └─────────────────┘           └─────────────────┘     │
│                                                         │
│  TCP 握手耗时 (ms)              带宽趋势 (Mbps)           │
│  ┌─────────────────┐           ┌─────────────────┐     │
│  │   折线图          │           │   柱状图          │     │
│  │                  │           │                  │     │
│  └─────────────────┘           └─────────────────┘     │
│                                                         │
├─────────────────────────────────────────────────────────┤
│  该链路告警历史                                           │
│  时间           类型    状态      持续时长                  │
│  03-31 15:30   延迟    🔴未恢复   持续中                  │
│  03-30 08:12   丢包    🟢已恢复   23分钟                  │
└─────────────────────────────────────────────────────────┘
```

### 6.4 告警页面

**路由：** `/alerts`

```
┌─────────────────────────────────────────────────────────┐
│  告警记录                                                │
├─────────────────────────────────────────────────────────┤
│  筛选: [链路▾] [类型▾] [状态: 全部/未恢复/已恢复]           │
├─────────────────────────────────────────────────────────┤
│  时间              链路                 类型  状态    持续  │
│  03-31 15:30:00   入口HK→中转B        延迟  🔴未恢复 2h   │
│  03-31 14:12:00   中转A→落地2          丢包  🟢已恢复 23m  │
│  03-30 22:01:00   中转B→落地X          TCP  🟢已恢复 5m   │
│  ...                                                    │
└─────────────────────────────────────────────────────────┘
```

点击行 → 跳转对应链路详情页。

---

## 7. Server 配置扩展

在现有 `config.yaml` 中新增 `probe` 段：

```yaml
server:
  host: 0.0.0.0
  port: 8080

database:
  driver: postgres
  dsn: "..."

# ... 现有配置 ...

# 新增：链路监控配置
probe:
  enabled: true                    # 是否启用探针模块
  retention_days: 30               # 历史数据保留天数

  telegram:
    bot_token: ""                  # 留空则不发送通知
    chat_id: ""

  alert_rules:
    latency_threshold_ms: 200
    packet_loss_threshold_percent: 5
    tcp_connect_threshold_ms: 500
    bandwidth_threshold_mbps: 10
    offline_timeout_seconds: 180
    recovery_count: 3              # 恢复需要的连续正常次数
```

---

## 8. 项目目录结构变更

在现有 tolato 目录结构上新增部分（标 `+` 的是新增）：

```
tolato/
├── cmd/
│   ├── server/
│   │   └── main.go
│   └── agent/
│       └── main.go
├── internal/
│   ├── server/
│   │   ├── handler/
│   │   │   ├── api.go
│   │   │   ├── chat_ws.go
│   │   │   ├── agent_ws.go
│   │   │   └── probe_api.go       + 探针 HTTP handlers
│   │   ├── agent/
│   │   ├── node/
│   │   ├── model/
│   │   ├── store/
│   │   └── probe/                  + 探针 server 模块
│   │       ├── store.go            +   数据库操作
│   │       ├── alert.go            +   告警引擎
│   │       ├── telegram.go         +   Telegram 通知
│   │       └── types.go            +   数据结构
│   └── agent/
│       ├── client/
│       ├── executor/
│       ├── collector/
│       └── probe/                  + 探针采集模块
│           ├── scheduler.go        +   定时调度（30s/5min）
│           ├── ping.go             +   ICMP Ping
│           ├── tcp.go              +   TCP 握手计时
│           ├── bandwidth.go        +   HTTP 下载测速
│           ├── reporter.go         +   HTTP 上报
│           └── fileserver.go       +   带宽测试文件服务
├── web/
│   └── src/
│       ├── views/
│       │   ├── MonitorView.vue     + 拓扑总览页
│       │   ├── LinkDetailView.vue  + 链路详情页
│       │   └── AlertsView.vue      + 告警列表页
│       ├── components/
│       │   └── monitor/            + 监控相关组件
│       │       ├── TopologyCanvas.vue  + 拓扑画布
│       │       ├── NodeCard.vue    + 节点卡片
│       │       ├── LinkLine.vue    + 链路连线
│       │       └── MetricChart.vue + 指标图表
│       └── stores/
│           └── monitor.ts          + 监控状态管理
├── db/
│   └── migrations/
│       └── 000x_probe_tables.sql   + 探针相关建表
└── configs/
    └── config.yaml                   扩展 probe 配置段
```

---

## 9. 开发优先级

在现有 Phase 排期之后新增：

### Phase 5（原 Phase 5 改为 Phase 6）— 链路监控

**P0 — 核心闭环：**
1. Agent 探针模块：ping + tcp + 上报
2. Server 接收存储指标
3. 前端监控首页拓扑图 + 链路状态

**P1 — 告警与详情：**
4. 告警引擎 + Telegram 通知
5. 链路详情页历史趋势图
6. 告警列表页

**P2 — 完善：**
7. 带宽测速（agent fileserver + bandwidth probe）
8. 数据自动清理
9. Server 下发探针配置（替代 agent 本地配置）
10. 节点离线检测
