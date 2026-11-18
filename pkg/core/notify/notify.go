package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/ashanzzz/personal-vps-monitor/pkg/core/config"
)

var httpClient = &http.Client{
	Timeout: 10 * time.Second,
}

func SendTelegram(text string) error {
	cfg := config.Global
	if cfg.TelegramBotToken == "" || cfg.TelegramChatID == "" {
		log.Printf("[Notify] Telegram credentials missing, skipped: %s", text)
		return nil
	}

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", cfg.TelegramBotToken)
	payload := map[string]interface{}{
		"chat_id":    cfg.TelegramChatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("telegram api error: status %d", resp.StatusCode)
	}

	return nil
}
