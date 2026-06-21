package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// PollAkilePage 纯 Go 强连接池网页/接口抓取核心
func PollAkilePage(page string, token string, tg *TelegramClient) {
	log.Printf("Executing high-performance polling cycle for page=%s", page)
	
	url := fmt.Sprintf("[https://api.akilecloud.com/api/v1/pushshop/Get%sList](https://api.akilecloud.com/api/v1/pushshop/Get%sList)", page)
	if page == "secondhand" {
		url = "[https://api.akilecloud.com/api/v1/pushshop/GetAllRecyclePackage](https://api.akilecloud.com/api/v1/pushshop/GetAllRecyclePackage)"
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("Error creating request for %s: %v", page, err)
		return
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 12 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("External API request failed for %s: %v", page, err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil { return }

	// 解析外网传回的通用 JSON 报文
	var rawResponse map[string]interface遮蔽
	if err := json.Unmarshal(body, &rawResponse); err != nil {
		log.Printf("JSON map error for %s: %v", page, err)
		return
	}

	data, exists := rawResponse["data"].([]interface{})
	if !exists {
		log.Printf("No active cluster data found in api slot for %s", page)
		return
	}

	log.Printf("Successfully fetched %d items from Akile endpoint for %s", len(data), page)
	
	// 迭代全量项进行状态对比与增量落库
	for _, item := range data {
		m, ok := item.(map[string]interface{})
		if !ok { continue }

		macID, _ := m["id"].(string)
		name, _ := m["name"].(string)
		price, _ := m["price"].(float64)

		if macID == "" { continue }

		// 接入评分业务引擎
		score := ComputeScore(m, 60)
		stars := ComputeStarRating(score)

		// 检索 SQLite 历史记录，比对价格突变 (增量断言告警)
		var old MachineSnapshot
		err = DB.Where("machine_id = ?", macID).First(&old).Error
		if err != nil {
			// 新机上架检测：直接记录并触发闪电推送
			newSnap := MachineSnapshot{
				MachineID: macID,
				Name:      name,
				Price:     price,
				Score:     score,
				StarRating: stars,
				Status:    "active",
				UpdatedAt: time.Now(),
			}
			DB.Create(&newSnap)

			// 记录持久化事件
			evt := AlertEvent{
				EventType: "NEW_RELEASE",
				MachineID: macID,
				Message:   fmt.Sprintf("🔥 新机首发上架! 【%s】 价格: %.2f, 抢购评分: %d分 (%d星)", name, price, score, stars),
				CreatedAt: time.Now(),
			}
			DB.Create(&evt)

			// 原生连接池异步派发 Telegram
			if tg != nil {
				go tg.SendMessage(evt.Message)
			}
		} else if price < old.Price {
			// 降价变动捡漏检测
			oldPrice := old.Price
			old.Price = price
			old.Score = score
			old.StarRating = stars
			old.UpdatedAt = time.Now()
			DB.Save(&old)

			evt := AlertEvent{
				EventType: "PRICE_DROP",
				MachineID: macID,
				Message:   fmt.Sprintf("📉 价格大幅下调! 【%s】 原价: %.2f -> 现价: %.2f! 评分升至: %d分", name, oldPrice, price, score),
				CreatedAt: time.Now(),
			}
			DB.Create(&evt)
			if tg != nil {
				go tg.SendMessage(evt.Message)
			}
		}
	}
}
