package probe

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/momaek/tolato/server/internal/model"
)

// TelegramNotifier sends alerts via Telegram Bot API.
type TelegramNotifier struct {
	botToken string
	chatID   string
	client   *http.Client
}

// NewTelegramNotifier creates a new TelegramNotifier.
// Returns nil if botToken or chatID is empty (disabled).
func NewTelegramNotifier(botToken, chatID string) *TelegramNotifier {
	if botToken == "" || chatID == "" {
		return nil
	}
	return &TelegramNotifier{
		botToken: botToken,
		chatID:   chatID,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (tn *TelegramNotifier) SendAlert(alert *model.ProbeAlert, linkName string) error {
	text := fmt.Sprintf(
		"🔴 告警：链路异常\n━━━━━━━━━━━━━\n链路：%s\n类型：%s\n详情：%s\n时间：%s",
		linkName,
		alert.Type,
		alert.Message,
		alert.TriggeredAt.Format("2006-01-02 15:04:05 MST"),
	)
	return tn.send(text)
}

func (tn *TelegramNotifier) SendRecovery(alert *model.ProbeAlert, linkName string, duration time.Duration) error {
	text := fmt.Sprintf(
		"🟢 恢复：链路恢复正常\n━━━━━━━━━━━━━\n链路：%s\n类型：%s 恢复\n持续时间：%s\n时间：%s",
		linkName,
		alert.Type,
		formatDuration(duration),
		time.Now().Format("2006-01-02 15:04:05 MST"),
	)
	return tn.send(text)
}

func (tn *TelegramNotifier) send(text string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", tn.botToken)
	resp, err := tn.client.PostForm(apiURL, url.Values{
		"chat_id": {tn.chatID},
		"text":    {text},
	})
	if err != nil {
		log.Printf("[telegram] send failed: %v", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Printf("[telegram] API returned %d", resp.StatusCode)
		return fmt.Errorf("telegram API returned %d", resp.StatusCode)
	}
	return nil
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d 秒", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d 分钟", int(d.Minutes()))
	}
	return fmt.Sprintf("%.1f 小时", d.Hours())
}
