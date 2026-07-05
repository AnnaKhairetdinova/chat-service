package handlers

import (
	"chat-app/database"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func CreateDirectChat(c *gin.Context) {
	userUUIDStr := c.GetString("user_uuid")
	userUUID, err := uuid.Parse(userUUIDStr)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid user uuid"})
		return
	}

	var input struct {
		WithEmail string    `json:"with_email"`
		WithUUID  uuid.UUID `json:"with"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "нужен параметр with_email или with"})
		return
	}

	var otherUserUUID uuid.UUID

	if input.WithEmail != "" {
		err = database.DB.QueryRow(`
SELECT uuid FROM users WHERE email = $1
`, input.WithEmail).Scan(&otherUserUUID)

		if err != nil {
			c.JSON(400, gin.H{"err": "пользователь с таким email не найден"})
			return
		}
	} else if input.WithUUID != uuid.Nil {
		otherUserUUID = input.WithUUID
	} else {
		c.JSON(400, gin.H{"err": "нельзя писать себе"})
		return
	}

	// Проверяем, существует ли уже такой чат
	var existingChatUUID string
	participants := []string{userUUIDStr, otherUserUUID.String()}
	participantsJSON, _ := json.Marshal(participants)

	err = database.DB.QueryRow(`
		SELECT uuid FROM chats
		WHERE type = 'direct'
		AND participants = $1
	`, string(participantsJSON)).Scan(&existingChatUUID)

	if err == nil {
		c.JSON(200, gin.H{"chat_uuid": existingChatUUID, "message": "чат уже существует"})
		return
	}

	chatUUID := uuid.New()

	_, err = database.DB.Exec(`
		INSERT INTO chats (uuid, type, participants, creator_uuid, created_at, updated_at)
		VALUES ($1, 'direct', $2, $3, $4, $4)
	`, chatUUID, string(participantsJSON), userUUID, time.Now())

	if err != nil {
		c.JSON(500, gin.H{"error": "не удалось создать чат"})
		return
	}

	c.JSON(200, gin.H{"chat_uuid": chatUUID})
}

func CreateGroupChat(c *gin.Context) {
	userUUIDStr := c.GetString("user_uuid")
	userUUID, err := uuid.Parse(userUUIDStr)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid user uuid"})
		return
	}

	var input struct {
		Name         string   `json:"name" binding:"required,min=1,max=100"`
		Participants []string `json:"participants,omitempty"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(400, gin.H{"error": "нужно имя группы"})
		return
	}

	chatUUID := uuid.New()
	participants := []string{userUUIDStr}

	if input.Participants != nil && len(input.Participants) > 0 {
		for _, p := range input.Participants {
			if p != userUUIDStr && p != "" {
				var exists bool

				err := database.DB.QueryRow(`
SELECT EXISTS(SELECT 1 FROM users WHERE uuid = $1)
`, p).Scan(&exists)

				if err == nil && exists {
					participants = append(participants, p)
				}
			}
		}
	}

	participantsJSON, _ := json.Marshal(participants)

	_, err = database.DB.Exec(`
		INSERT INTO chats (uuid, type, name, participants, creator_uuid, created_at, updated_at)
		VALUES ($1, 'group', $2, $3, $4, $5, $5)
	`, chatUUID, input.Name, string(participantsJSON), userUUID, time.Now())

	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"chat_uuid": chatUUID,
		"name":      input.Name,
	})
}

func GetUserChats(c *gin.Context) {
	userUUID := c.GetString("user_uuid")

	rows, err := database.DB.Query(`
		SELECT uuid, type, name, participants, created_at
		FROM chats
		WHERE participants::jsonb ? $1
		ORDER BY created_at DESC
	`, userUUID)

	if err != nil {
		c.JSON(500, gin.H{
			"error":     "db error",
			"details":   err.Error(),
			"user_uuid": userUUID,
		})
		return
	}
	defer rows.Close()

	var chats []map[string]any

	for rows.Next() {
		var chatUUID uuid.UUID
		var chatType string
		var name sql.NullString
		var participantsJSON string
		var createdAt time.Time

		if err := rows.Scan(&chatUUID, &chatType, &name, &participantsJSON, &createdAt); err != nil {
			continue
		}

		chat := map[string]any{
			"chat_uuid":  chatUUID.String(),
			"type":       chatType,
			"created_at": createdAt,
		}

		if name.Valid {
			chat["name"] = name.String
		}

		if chatType == "direct" {
			var participants []string
			if err := json.Unmarshal([]byte(participantsJSON), &participants); err == nil {
				var otherUserUUID string
				for _, p := range participants {
					if p != userUUID {
						otherUserUUID = p
						break
					}
				}

				if otherUserUUID != "" {
					var otherName, otherSurname sql.NullString
					err := database.DB.QueryRow(`
						SELECT name, surname FROM users WHERE uuid = $1
					`, otherUserUUID).Scan(&otherName, &otherSurname)

					if err == nil {
						var fullName string
						if otherName.Valid && otherSurname.Valid {
							fullName = otherName.String + " " + otherSurname.String
						} else if otherName.Valid {
							fullName = otherName.String
						} else if otherSurname.Valid {
							fullName = otherSurname.String
						} else {
							fullName = "Пользователь"
						}
						chat["participant_name"] = fullName
					}
				}
			}
		}

		chats = append(chats, chat)
	}

	c.JSON(200, gin.H{"chats": chats})
}

func GetChatMessages(c *gin.Context) {
	userUUID := c.GetString("user_uuid")
	chatUUIDStr := c.Param("chat_uuid")

	chatUUID, err := uuid.Parse(chatUUIDStr)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid chat uuid"})
		return
	}

	// Проверяем доступ к чату
	var count int
	err = database.DB.QueryRow(`
		SELECT COUNT(*) FROM chats
		WHERE uuid = $1 AND participants::jsonb ? $2
	`, chatUUID, userUUID).Scan(&count)

	if err != nil || count == 0 {
		c.JSON(403, gin.H{"error": "access denied"})
		return
	}

	rows, err := database.DB.Query(`
		SELECT uuid, sender_uuid, sender_name, content, created_at, is_read
		FROM messages
		WHERE chat_uuid = $1
		ORDER BY created_at ASC
	`, chatUUID)

	if err != nil {
		c.JSON(500, gin.H{"error": "db error"})
		return
	}
	defer rows.Close()

	var messages []map[string]interface{}
	for rows.Next() {
		var msgUUID uuid.UUID
		var senderUUID uuid.UUID
		var senderName, content string
		var createdAt time.Time
		var isRead bool

		if err := rows.Scan(&msgUUID, &senderUUID, &senderName, &content, &createdAt, &isRead); err != nil {
			continue
		}

		messages = append(messages, map[string]interface{}{
			"uuid":        msgUUID.String(),
			"chat_uuid":   chatUUIDStr,
			"sender_uuid": senderUUID.String(),
			"sender_name": senderName,
			"content":     content,
			"created_at":  createdAt,
			"is_read":     isRead,
		})
	}

	c.JSON(200, gin.H{"messages": messages})
}

func SearchUsers(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(400, gin.H{"error": "query parameter required"})
		return
	}

	rows, err := database.DB.Query(`
		SELECT uuid, name, surname, email
		FROM users
		WHERE email ILIKE $1 OR name ILIKE $1 OR surname ILIKE $1
		LIMIT 20
	`, "%"+query+"%")

	if err != nil {
		c.JSON(500, gin.H{"error": "db error"})
		return
	}
	defer rows.Close()

	var users []map[string]interface{}
	for rows.Next() {
		var uuid, name, surname, email string
		if err := rows.Scan(&uuid, &name, &surname, &email); err != nil {
			continue
		}

		fullName := name
		if surname != "" {
			fullName = name + " " + surname
		}

		users = append(users, map[string]interface{}{
			"uuid":  uuid,
			"name":  fullName,
			"email": email,
		})
	}

	c.JSON(200, gin.H{"users": users})
}

func MarkChatAsRead(c *gin.Context) {
	userUUID := c.GetString("user_uuid")
	chatUUIDStr := c.Param("chat_uuid")

	chatUUID, err := uuid.Parse(chatUUIDStr)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid chat uuid"})
		return
	}

	var count int
	err = database.DB.QueryRow(`
		SELECT COUNT(*) FROM chats
		WHERE uuid = $1 AND participants::jsonb ? $2
	`, chatUUID, userUUID).Scan(&count)

	if err != nil || count == 0 {
		c.JSON(403, gin.H{"error": "access denied"})
		return
	}

	_, err = database.DB.Exec(`
		UPDATE messages
		SET is_read = true, updated_at = NOW()
		WHERE chat_uuid = $1
		AND sender_uuid != $2
		AND is_read = false
	`, chatUUID, userUUID)

	if err != nil {
		c.JSON(500, gin.H{"error": "failed to mark as read"})
		return
	}

	c.JSON(200, gin.H{"message": "marked as read"})
}
