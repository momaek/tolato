package agent

import (
	"fmt"
	"strings"
)

// NodeInfo holds the information about a registered node that the prompt
// builder uses to render the "current online nodes" section.
type NodeInfo struct {
	ID     string
	Name   string // hostname
	Alias  string // user-defined alias (may be empty)
	IP     string
	OS     string
	Status string // "online" or "offline"
}

// PromptBuilder builds the system prompt for the AI agent loop.
type PromptBuilder struct{}

// NewPromptBuilder creates a new PromptBuilder.
func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{}
}

// Build constructs the full system prompt with dynamic node info.
// customPrompt is the user-configured custom system prompt (may be empty).
// defaultNodeID, if non-empty, marks the node the user pre-selected in the
// composer; the LLM is told to use it as the implicit target when the user
// says "this node" / "该节点" without naming one explicitly.
func (pb *PromptBuilder) Build(nodes []NodeInfo, customPrompt string, defaultNodeID string) string {
	var b strings.Builder

	pb.writeRole(&b)
	pb.writeTools(&b)
	pb.writeNodes(&b, nodes)
	pb.writeDefaultNode(&b, nodes, defaultNodeID)
	pb.writeSecurityRules(&b)
	pb.writeCustomPrompt(&b, customPrompt)

	return b.String()
}

func (pb *PromptBuilder) writeRole(b *strings.Builder) {
	b.WriteString(`你是 Tolato，一个专业的服务器运维 AI 助手。你可以通过工具来管理用户的 VPS 服务器。

请遵循以下原则：
- 在执行命令前，向用户解释你打算做什么
- 如果命令可能有风险，提醒用户
- 执行完命令后，解读输出结果并给出建议
- 使用 Markdown 格式输出，代码块使用适当的语言标记

`)
}

func (pb *PromptBuilder) writeTools(b *strings.Builder) {
	b.WriteString(`## 可用工具

### list_nodes
列出所有已注册的 VPS 节点及其状态。无需参数。

### get_node_info
获取指定节点的详细系统信息和实时指标。
参数：
- node_id (string, required): 节点 ID

### execute_command
在指定 VPS 上执行 Shell 命令。
参数：
- node_id (string, required): 目标节点 ID
- command (string, required): 要执行的命令
- timeout (integer, optional): 超时时间（秒），默认 60

`)
}

func (pb *PromptBuilder) writeNodes(b *strings.Builder, nodes []NodeInfo) {
	b.WriteString("## 当前在线节点\n\n")

	online := make([]NodeInfo, 0, len(nodes))
	for _, n := range nodes {
		if n.Status == "online" {
			online = append(online, n)
		}
	}

	if len(online) == 0 {
		b.WriteString("当前没有在线节点。\n\n")
		return
	}

	b.WriteString("| ID | 名称 | 别名 | IP | 系统 |\n")
	b.WriteString("|----|------|------|----|------|\n")
	for _, n := range online {
		alias := n.Alias
		if alias == "" {
			alias = "-"
		}
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s |\n", n.ID, n.Name, alias, n.IP, n.OS)
	}
	b.WriteString("\n")
}

func (pb *PromptBuilder) writeDefaultNode(b *strings.Builder, nodes []NodeInfo, defaultNodeID string) {
	if defaultNodeID == "" {
		return
	}
	var match *NodeInfo
	for i := range nodes {
		if nodes[i].ID == defaultNodeID {
			match = &nodes[i]
			break
		}
	}
	if match == nil {
		return
	}
	label := match.Name
	if match.Alias != "" {
		label = match.Alias
	}
	fmt.Fprintf(b, "## 用户当前选中的节点\n\n用户在对话框中预选了以下节点作为默认操作目标：\n- ID: %s\n- 名称: %s (%s)\n\n当用户使用「这个节点」「该节点」「当前节点」等指代但未明确点名时，默认使用此节点的 ID 调用工具，无需再追问。仅当用户明确指定了其他节点名称/别名/编号时才切换目标。\n\n", match.ID, label, match.IP)
}

func (pb *PromptBuilder) writeSecurityRules(b *strings.Builder) {
	b.WriteString(`## 安全规则
- 涉及敏感操作（如删除文件、重启服务、修改系统配置）时，系统会自动触发确认流程
- 不要尝试执行被禁止的命令
- 优先使用安全的命令方式（如 rm 前先 ls 确认）

`)
}

func (pb *PromptBuilder) writeCustomPrompt(b *strings.Builder, customPrompt string) {
	trimmed := strings.TrimSpace(customPrompt)
	if trimmed == "" {
		return
	}
	b.WriteString("## 自定义指令\n\n")
	b.WriteString(trimmed)
	b.WriteString("\n")
}
