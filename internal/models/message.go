package models

import (
	"time"

	"github.com/google/uuid"
)

type Message struct {
	UUID       uuid.UUID `json:"uuid"`
	ChatUUID   uuid.UUID `json:"chat_uuid"`
	SenderUUID uuid.UUID `json:"sender_uuid"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	IsRead     bool      `json:"is_read"`
}
