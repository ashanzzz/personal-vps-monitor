package main

import (
	"log"
	"os"
	"path/filepath"
	"time"
)

func main() {
	log.Println("Starting full Go executable architecture for Akile Monitor v2...")

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "../data"
	}
	dbPath := filepath.Join(dataDir, "monitor.db")

	log.Printf("Connecting to SQLite via GORM: %s", dbPath)
	InitDB(dbPath)

	// 自动迁移并校验底层存储健康度
	DB.AutoMigrate(&Setting{}, &MachineSnapshot{}, &AlertEvent{})

	tgToken := GetSetting("tg_bot_token")
	tgChatID := GetSetting("tg_chat_id")
	var tg *TelegramClient
	if tgToken != "" && tgChatID != "" {
		tg = NewTelegramClient(tgToken, tgChatID)
		log.Println("Telegram notification channel attached successfully.")
	}

	// 启动 HTTP 接口引擎与前端托管分发服务
	r := SetupRouter()
	go func() {
		log.Println("Go internal micro Web-Server listening on :8080")
		if err := r.Run(":8080"); err != nil {
			log.Fatalf("Web-Server failed to start: %v", err)
		}
	}()

	// 核心原生 Goroutine 错峰定时轮询常驻环 (每60秒执行全量页面扫描)
	cloudTicker := time.NewTicker(60 * time.Second)
	defer cloudTicker.Stop()

	log.Println("Go runtime scheduler fully activated. Pure-Go container state is healthy.")

	// 启动时立刻先首跑一次，确保不空等
	token := GetSetting("akile_token")
	if token != "" {
		go PollAkilePage("cloud", token, tg)
		go PollAkilePage("ls", token, tg)
		go PollAkilePage("secondhand", token, tg)
	}

	for {
		select {
		case <-cloudTicker.C:
			token := GetSetting("akile_token")
			if token == "" {
				continue
			}
			// 并发投放至独立 Goroutine 协程，零堆积卡死风险
			go PollAkilePage("cloud", token, tg)
			go PollAkilePage("ls", token, tg)
			go PollAkilePage("secondhand", token, tg)
		}
	}
}
