# NodeProbe — 加速链路监控系统需求文档

## 一、项目概述

构建一套**自建的链路监控系统**，用于监控加速网络中各节点之间的链路质量。系统由两部分组成：

1. **Agent（探针）**：部署在每个节点上，定时对下游节点执行测速任务，上报结果
2. **Server（中心端）**：接收 Agent 上报的数据，存储、展示、告警

---

## 二、网络拓扑

```
                     ┌─── 落地节点 1
                     ├─── 落地节点 2
    入口 ──→ 中转 A ──┼─── 落地节点 3
      │              ├─── ...
      │              └─── 落地节点 N
      │
      │              ┌─── 落地节点 X
      └───→ 中转 B ──┼─── 落地节点 Y
                     ├─── ...
                     └─── 落地节点 Z
```

- **入口节点**：1 个（测试目标：中转 A、中转 B）
- **中转节点**：2 个（测试目标：各自负责的落地节点）
- **落地节点**：10+ 个（被测试目标，不主动测试其他节点）
- 所有节点均为 **Linux** 系统

---

## 三、技术栈

| 组件 | 技术选型 |
|------|---------|
| 后端语言 | **Go** |
| 数据库 | **SQLite**（单文件，轻量部署） |
| 前端 | 自建 Web Dashboard（内嵌在 Go 服务中，可用模板引擎或前后端分离） |
| Agent | Go 编译为单二进制，部署到各节点 |
| 告警 | **Telegram Bot** |

---

## 四、监控指标

Agent 对每条链路（当前节点 → 目标节点）采集以下 4 项指标：

### 4.1 延迟（Latency）
- 方式：ICMP Ping 或 TCP Ping
- 记录：最小值、平均值、最大值（单位 ms）
- 采集频率：每 **30 秒**

### 4.2 丢包率（Packet Loss）
- 方式：连续发送 N 个 Ping 包，统计丢失百分比
- 记录：丢包率百分比
- 采集频率：每 **30 秒**（与延迟同一次 Ping 任务一起采集）

### 4.3 TCP 连接耗时（TCP Handshake Time）
- 方式：对目标节点指定端口发起 TCP 连接，记录三次握手耗时
- 记录：连接耗时（单位 ms）
- 采集频率：每 **30 秒**

### 4.4 带宽 / 吞吐量（Bandwidth）
- 方式：HTTP 下载测速（目标节点上跑一个简单的 HTTP 文件服务，Agent 下载固定大小的文件计算速度）或 iperf3
- 记录：下载速度（单位 Mbps）
- 采集频率：每 **5 分钟**（带宽测试开销大，频率降低）

---

## 五、系统架构

### 5.1 Agent（部署在入口节点、中转节点上）

```
agent 二进制
├── 读取配置文件（YAML/JSON）
│   ├── server_url: 中心端上报地址
│   ├── node_id: 当前节点标识
│   ├── node_name: 当前节点名称
│   └── targets: 目标节点列表
│       ├── { id, name, host, ping_port, tcp_port, bandwidth_url }
│       └── ...
├── 定时任务
│   ├── 每 30s: ping + tcp connect 测试
│   └── 每 5min: 带宽测试
└── 上报数据到 Server（HTTP POST JSON）
```

**Agent 配置文件示例（agent.yaml）：**
```yaml
server_url: "http://<server-ip>:8080/api/report"
auth_token: "xxx"  # 简单的 Bearer Token 认证
node_id: "entry-1"
node_name: "入口-香港"

targets:
  - id: "relay-a"
    name: "中转A-东京"
    host: "10.0.1.1"
    ping_count: 10          # 每次发 10 个包
    tcp_port: 443           # TCP 握手测试端口
    bandwidth_url: "http://10.0.1.1:9090/testfile"  # 带宽测试下载地址
  - id: "relay-b"
    name: "中转B-新加坡"
    host: "10.0.2.1"
    ping_count: 10
    tcp_port: 443
    bandwidth_url: "http://10.0.2.1:9090/testfile"
```

### 5.2 Server（中心端，单机部署）

```
server 二进制
├── HTTP API
│   ├── POST /api/report          — Agent 上报数据（带 Token 认证）
│   ├── GET  /api/nodes           — 获取所有节点信息
│   ├── GET  /api/links           — 获取所有链路及最新状态
│   ├── GET  /api/links/:id/history — 获取某条链路的历史数据（支持时间范围查询）
│   └── GET  /api/alerts          — 获取告警记录
├── Web Dashboard（前端页面）
│   ├── 首页：拓扑总览 + 所有链路状态卡片
│   ├── 链路详情页：某条链路的历史趋势图（延迟、丢包、带宽、TCP 耗时）
│   └── 告警页：历史告警记录列表
├── 告警引擎
│   ├── 检查每次上报数据是否触发阈值
│   ├── 触发后发送 Telegram 消息
│   └── 支持告警恢复通知
├── 数据清理
│   └── 定时清理超过 N 天的历史数据（可配置，默认 30 天）
└── SQLite 数据库
```

---

## 六、数据库设计（SQLite）

### 表：nodes（节点信息）
| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT PK | 节点 ID |
| name | TEXT | 节点名称 |
| role | TEXT | 角色：entry / relay / landing |
| last_seen | DATETIME | 最后上报时间 |

### 表：links（链路定义）
| 字段 | 类型 | 说明 |
|------|------|------|
| id | TEXT PK | 链路 ID（格式：source_id -> target_id） |
| source_id | TEXT FK | 源节点 |
| target_id | TEXT FK | 目标节点 |

### 表：metrics（监控数据，主表）
| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | 自增 |
| link_id | TEXT FK | 链路 ID |
| timestamp | DATETIME | 采集时间 |
| latency_min | REAL | 最小延迟 ms |
| latency_avg | REAL | 平均延迟 ms |
| latency_max | REAL | 最大延迟 ms |
| packet_loss | REAL | 丢包率 % |
| tcp_connect_time | REAL | TCP 握手耗时 ms |
| bandwidth_mbps | REAL | 带宽 Mbps（可为 NULL，非每次都测） |

### 表：alerts（告警记录）
| 字段 | 类型 | 说明 |
|------|------|------|
| id | INTEGER PK | 自增 |
| link_id | TEXT FK | 链路 ID |
| type | TEXT | 告警类型：latency / packet_loss / tcp / bandwidth / offline |
| message | TEXT | 告警内容 |
| triggered_at | DATETIME | 触发时间 |
| resolved_at | DATETIME | 恢复时间（NULL 表示未恢复） |

**索引建议：**
- metrics 表：`(link_id, timestamp)` 联合索引
- alerts 表：`(link_id, triggered_at)` 联合索引

---

## 七、告警规则

### 7.1 默认告警阈值（Server 配置文件可调）

| 指标 | 告警条件 | 恢复条件 |
|------|---------|---------|
| 延迟 | 平均延迟 > **200ms** | 连续 3 次 < 150ms |
| 丢包率 | 丢包 > **5%** | 连续 3 次 < 1% |
| TCP 耗时 | 握手 > **500ms** | 连续 3 次 < 300ms |
| 带宽 | 速度 < **10Mbps** | 连续 2 次 > 20Mbps |
| 节点离线 | 超过 **3 分钟**未上报 | 恢复上报 |

### 7.2 Telegram 告警格式

```
🔴 告警：链路异常
━━━━━━━━━━━━━
链路：入口-香港 → 中转A-东京
类型：延迟过高
当前值：延迟 356ms（阈值 200ms）
时间：2026-03-31 15:30:00 UTC+8
```

```
🟢 恢复：链路恢复正常
━━━━━━━━━━━━━
链路：入口-香港 → 中转A-东京
类型：延迟恢复
当前值：延迟 45ms
持续时间：12 分钟
时间：2026-03-31 15:42:00 UTC+8
```

---

## 八、Web Dashboard 页面设计

### 8.1 首页 — 画布拓扑总览

**顶部状态栏：** 4 个统计卡片横排（总链路数、正常数、警告数、告警数）

**中间核心区域：画布拓扑图（Canvas Topology）**

这是首页的核心。在一个大画布区域上，按照实际网络拓扑从左到右排列所有节点：

```
布局方式（从左到右三列）：

[入口节点]  ──→  [中转节点]  ──→  [落地节点]
  (1个)           (2个)           (10+个)
  左侧             中间              右侧
```

**节点卡片（Node Card）：**
- 卡片保持简洁，只显示：状态指示灯（绿/黄/红）+ 节点名称 + 角色标签（入口/中转/落地）
- 卡片上不显示任何指标数据
- 点击节点卡片 → 跳转到该节点相关的所有链路详情

**连接线（Link Line）：**
- 节点之间用贝塞尔曲线连接，带箭头表示数据流方向
- 连线颜色反映链路状态：绿色=正常、黄色=警告、红色=告警
- **连线中间有小标签**，直接标注该链路的核心指标摘要（如 `32ms | 0%`，显示延迟和丢包率）
- **鼠标悬浮标签**弹出 tooltip 浮层，展示完整四项指标：
  - 链路名称（如：入口-香港 → 中转A-东京）
  - 延迟（ms）
  - 丢包率（%）
  - TCP 连接耗时（ms）
  - 带宽（Mbps）
- 点击连线标签 → 跳转到该链路的详情页

**画布交互：**
- 支持鼠标滚轮缩放（节点多时不拥挤）
- 支持拖拽平移画布
- 节点位置可由后端根据拓扑自动计算（三列布局：入口在左、中转在中、落地在右）
- 落地节点按所属中转分组，上下排列

**底部：** 最近告警列表（最近 10 条），每条显示时间、链路、类型、当前值

### 8.2 链路详情页

- 链路基本信息（源节点 → 目标节点）
- **4 个趋势图**（支持选择时间范围：1h / 6h / 24h / 7d）：
  - 延迟趋势（折线图，展示 min/avg/max）
  - 丢包率趋势（面积图）
  - TCP 连接耗时趋势（折线图）
  - 带宽趋势（柱状图）
- 该链路的告警历史

### 8.3 告警页

- 告警列表：时间、链路、类型、状态（未恢复/已恢复）、持续时长
- 支持按链路、类型、状态筛选

---

## 九、Server 配置文件示例（server.yaml）

```yaml
listen: ":8080"
auth_token: "xxx"  # 与 Agent 配置一致

database:
  path: "./data/monitor.db"
  retention_days: 30  # 历史数据保留天数

telegram:
  bot_token: "123456:ABC-DEF..."
  chat_id: "-100123456789"

alert_rules:
  latency_threshold_ms: 200
  packet_loss_threshold_percent: 5
  tcp_connect_threshold_ms: 500
  bandwidth_threshold_mbps: 10
  offline_timeout_seconds: 180

  # 恢复需要的连续正常次数
  recovery_count: 3
```

---

## 十、部署方式

### 10.1 Server 部署
```bash
# 编译
go build -o nodeprobe-server ./cmd/server

# 运行
./nodeprobe-server -config server.yaml
```
- 单二进制 + 一个 YAML 配置 + 一个 SQLite 文件
- 前端静态文件 embed 到 Go 二进制中（go:embed）

### 10.2 Agent 部署
```bash
# 交叉编译
GOOS=linux GOARCH=amd64 go build -o nodeprobe-agent ./cmd/agent

# 在节点上运行
./nodeprobe-agent -config agent.yaml
```
- 单二进制 + 一个 YAML 配置
- 建议用 systemd 管理

### 10.3 带宽测速文件服务（落地节点 / 中转节点上可选）
- 每个被测试的节点上需要跑一个简单的 HTTP 文件服务
- 提供一个固定大小的测试文件（如 10MB）供 Agent 下载测速
- 可以做成 Agent 的一个子命令：`./nodeprobe-agent serve-testfile --port 9090 --size 10MB`

---

## 十一、项目结构建议

```
nodeprobe/
├── cmd/
│   ├── server/
│   │   └── main.go          # Server 入口
│   └── agent/
│       └── main.go          # Agent 入口
├── internal/
│   ├── server/
│   │   ├── api.go           # HTTP API handlers
│   │   ├── alert.go         # 告警引擎
│   │   ├── telegram.go      # Telegram Bot 发送
│   │   ├── db.go            # SQLite 操作
│   │   └── cleanup.go       # 数据清理
│   ├── agent/
│   │   ├── probe.go         # 测速探针（ping, tcp, bandwidth）
│   │   ├── reporter.go      # 数据上报
│   │   ├── scheduler.go     # 定时调度
│   │   └── fileserver.go    # 带宽测试文件服务
│   └── model/
│       └── types.go         # 共享数据结构
├── web/
│   ├── templates/           # HTML 模板（或前端 SPA）
│   ├── static/
│   │   ├── css/
│   │   └── js/
│   └── embed.go             # go:embed 嵌入前端资源
├── configs/
│   ├── server.yaml.example
│   └── agent.yaml.example
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## 十二、API 数据格式

### Agent 上报（POST /api/report）

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
    },
    {
      "target_id": "relay-b",
      "latency_min": 45.2,
      "latency_avg": 52.1,
      "latency_max": 68.9,
      "packet_loss": 1.0,
      "tcp_connect_time": 55.3,
      "bandwidth_mbps": 85.6
    }
  ]
}
```

### 响应
```json
{
  "status": "ok",
  "received": 2
}
```

---

## 十三、优先级排序

1. **P0 — 核心功能**：Agent 测速 + 上报、Server 接收存储、Dashboard 首页状态展示
2. **P1 — 重要功能**：Telegram 告警、链路详情历史趋势图、告警恢复通知
3. **P2 — 锦上添花**：数据自动清理、告警页筛选、Dashboard 美化、节点离线检测