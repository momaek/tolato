package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/momaek/tolato/internal/nodeprobe/model"
)

// TelegramNotifier sends alerts via Telegram Bot API.
type TelegramNotifier struct {
	BotToken string
	ChatID   string
	Client   *http.Client
}

// NewTelegramNotifier creates a Telegram notifier.
func NewTelegramNotifier(botToken, chatID string) *TelegramNotifier {
	return &TelegramNotifier{
		BotToken: botToken,
		ChatID:   chatID,
		Client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *TelegramNotifier) SendAlert(ctx context.Context, alert model.Alert, linkName string) error {
	typeName := alertTypeName(alert.Type)
	text := fmt.Sprintf(
		"\U0001F534 告警：链路异常\n━━━━━━━━━━━━━\n链路：%s\n类型：%s\n当前值：%s\n时间：%s",
		linkName, typeName, alert.Message,
		alert.TriggeredAt.Format("2006-01-02 15:04:05 MST"),
	)
	return t.sendMessage(ctx, text)
}

func (t *TelegramNotifier) SendRecovery(ctx context.Context, alert model.Alert, linkName string, duration time.Duration) error {
	typeName := alertTypeName(alert.Type) + "恢复"
	text := fmt.Sprintf(
		"\U0001F7E2 恢复：链路恢复正常\n━━━━━━━━━━━━━\n链路：%s\n类型：%s\n持续时间：%s\n时间：%s",
		linkName, typeName, formatDuration(duration),
		time.Now().Format("2006-01-02 15:04:05 MST"),
	)
	return t.sendMessage(ctx, text)
}

func (t *TelegramNotifier) sendMessage(ctx context.Context, text string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.BotToken)
	body, _ := json.Marshal(map[string]string{
		"chat_id": t.ChatID,
		"text":    text,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned %d", resp.StatusCode)
	}
	return nil
}

func alertTypeName(t model.AlertType) string {
	switch t {
	case model.AlertTypeLatency:
		return "延迟过高"
	case model.AlertTypePacketLoss:
		return "丢包率过高"
	case model.AlertTypeTCP:
		return "TCP连接超时"
	case model.AlertTypeBandwidth:
		return "带宽不足"
	case model.AlertTypeOffline:
		return "节点离线"
	default:
		return string(t)
	}
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%d小时%d分钟", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%d分钟%d秒", m, s)
	}
	return fmt.Sprintf("%d秒", s)
}

// NopNotifier is a no-op notifier used when Telegram is not configured.
type NopNotifier struct{}

func (NopNotifier) SendAlert(context.Context, model.Alert, string) error            { return nil }
func (NopNotifier) SendRecovery(context.Context, model.Alert, string, time.Duration) error { return nil }
