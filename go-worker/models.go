package main

import (
	"time"
)

type Setting struct {
	Key   string `gorm:"primaryKey;column:key" json:"key"`
	Value string `gorm:"column:value" json:"value"`
}

func (Setting) TableName() string {
	return "settings"
}

type MachineSnapshot struct {
	ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	MachineID     string    `gorm:"index;column:machine_id" json:"machine_id"`
	Name          string    `gorm:"column:name" json:"name"`
	Region        string    `gorm:"column:region" json:"region"`
	Price         float64   `gorm:"column:price" json:"price"`
	OriginalPrice float64   `gorm:"column:original_price" json:"original_price"`
	Score         int       `gorm:"column:score" json:"score"`
	StarRating    int       `gorm:"column:star_rating" json:"star_rating"`
	Status        string    `gorm:"column:status" json:"status"`
	UpdatedAt     time.Time `gorm:"column:updated_at" json:"updated_at"`
}

func (MachineSnapshot) TableName() string {
	return "machine_snapshots"
}

type AlertEvent struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	EventType string    `gorm:"column:event_type" json:"event_type"`
	MachineID string    `gorm:"column:machine_id" json:"machine_id"`
	Message   string    `gorm:"column:message" json:"message"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (AlertEvent) TableName() string {
	return "alert_events"
}
