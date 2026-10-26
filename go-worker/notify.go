package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type TelegramClient struct {
	BotToken string
	ChatID   string
	Client   *http.Client
}

func NewTelegramClient(token, chatID string) *TelegramClient {
	return &TelegramClient{
		BotToken: token,
		ChatID:   chatID,
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (tg *TelegramClient) SendMessage(text string) error {
	if tg.BotToken == "" || tg.ChatID == "" {
		return nil
	}

	url := fmt.Sprintf("[https://api.telegram.org/bot%s/sendMessage](https://api.telegram.org/bot%s/sendMessage)", tg.BotToken)
	payload := map[string]string{
		"chat_id":    tg.ChatID,
		"text":       text,
		"parse_mode": "Markdown",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := tg.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API returned status: %d", resp.StatusCode)
	}
	return nil
}
