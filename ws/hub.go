package ws

import (
	"chat-app/database"
	"chat-app/internal/redis"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var (
	upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
	JWTSecret []byte // <-- Глобальная переменная для секрета
)

type WMessage struct {
	UUID       string    `json:"uuid"`
	ChatUUID   string    `json:"chat_uuid"`
	ChatType   string    `json:"chat_type"`
	SenderUUID string    `json:"sender_uuid"`
	SenderName string    `json:"sender_name"`
	Content    string    `json:"content"`
	CreatedAt  time.Time `json:"created_at"`
	IsRead     bool      `json:"is_read"`
}

type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan WMessage
	userUUID uuid.UUID
	chatUUID string // добавлено для фильтрации сообщений
}

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan WMessage
	register   chan *Client
	unregister chan *Client
	mu         sync.Mutex
	userNames  sync.Map
}

var HubInstance = &Hub{
	clients:    make(map[*Client]bool),
	broadcast:  make(chan WMessage, 100),
	register:   make(chan *Client),
	unregister: make(chan *Client),
}

func SetJWTSecret(secret []byte) {
	JWTSecret = secret
}

func (h *Hub) Run() {
	go h.handleLocalBroadcast()
	go h.publishToRedis()
	go h.subscribeRedis()

	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
		}
	}
}

func (h *Hub) handleLocalBroadcast() {
	for msg := range h.broadcast {
		h.mu.Lock()
		for client := range h.clients {
			// Отправляем сообщение только клиентам, подключенным к этому чату
			if client.chatUUID == msg.ChatUUID || msg.ChatUUID == "global" {
				select {
				case client.send <- msg:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
		}
		h.mu.Unlock()
	}
}

func (h *Hub) publishToRedis() {
	ctx := context.Background()
	for msg := range h.broadcast {
		data, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		channel := "chat:" + msg.ChatUUID
		redis.Client.Publish(ctx, channel, data)
	}
}

func (h *Hub) subscribeRedis() {
	ctx := context.Background()
	pubsub := redis.Client.Subscribe(ctx, "chat:*")
	defer pubsub.Close()

	for msg := range pubsub.Channel() {
		var wmsg WMessage
		if err := json.Unmarshal([]byte(msg.Payload), &wmsg); err != nil {
			continue
		}
		h.broadcast <- wmsg
	}
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	for {
		var input struct {
			ChatUUID string `json:"chat_uuid"`
			Text     string `json:"text"` // потому что Vue отправляет "text"
		}

		err := c.conn.ReadJSON(&input)
		if err != nil {
			log.Println("read error:", err)
			break
		}

		log.Printf("Получено сообщение: chat_uuid='%s', content='%s'", input.ChatUUID, input.Text)

		if input.ChatUUID == "" || input.Text == "" {
			log.Printf("Пустое сообщение или chat_uuid!")
			continue
		}

		chatType := "direct"
		if strings.HasPrefix(input.ChatUUID, "group-") {
			chatType = "group"
		}

		senderName, ok := c.hub.getUserName(c.userUUID)
		if !ok {
			senderName = "пользователь"
		}

		msg := WMessage{
			UUID:       uuid.New().String(),
			ChatUUID:   input.ChatUUID,
			ChatType:   chatType,
			SenderUUID: c.userUUID.String(),
			SenderName: senderName,
			Content:    input.Text,
			CreatedAt:  time.Now(),
			IsRead:     false,
		}

		log.Printf("Создано WMessage: Content='%s'", msg.Content)

		saveMessageToDB(msg)
		c.hub.broadcast <- msg
	}
}

func (c *Client) writePump() {
	for msg := range c.send {
		err := c.conn.WriteJSON(msg)
		if err != nil {
			break
		}
	}
	c.conn.Close()
}

func HandleChat(c *gin.Context) {
	tokenString := c.Query("token")
	if tokenString == "" {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(401, gin.H{"error": "unauthorized"})
			return
		}
		tokenString = strings.TrimPrefix(authHeader, "Bearer ")
	}

	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return JWTSecret, nil
	}, jwt.WithValidMethods([]string{"HS256"}))

	if err != nil || !token.Valid {
		c.JSON(401, gin.H{"error": "invalid token"})
		return
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		c.JSON(401, gin.H{"error": "invalid claims"})
		return
	}

	userUUIDStr, ok := claims["user_uuid"].(string)
	if !ok {
		c.JSON(401, gin.H{"error": "invalid user_uuid in token"})
		return
	}

	userUUID, err := uuid.Parse(userUUIDStr)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid user uuid"})
		return
	}

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
	`, chatUUID, userUUIDStr).Scan(&count)

	if err != nil || count == 0 {
		c.JSON(403, gin.H{"error": "access denied to chat"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println("upgrade:", err)
		return
	}

	client := &Client{
		hub:      HubInstance,
		conn:     conn,
		send:     make(chan WMessage, 100),
		userUUID: userUUID,
		chatUUID: chatUUIDStr,
	}

	HubInstance.register <- client

	go func() {
		var name, surname string
		if err := database.DB.QueryRow(`
		SELECT COALESCE(name, ''), COALESCE(surname, ''), email
		FROM users WHERE uuid = $1
	`, client.userUUID).Scan(&name, &surname); err == nil {
			var fullName string
			if name != "" && surname != "" {
				fullName = name + " " + surname
			} else if name != "" {
				fullName = name
			} else if surname != "" {
				fullName = surname
			} else {
				fullName = "пользователь"
			}
			HubInstance.userNames.Store(client.userUUID, fullName)
		}
	}()

	rows, err := database.DB.Query(`
		SELECT uuid, sender_uuid, sender_name, content, created_at, is_read
		FROM messages
		WHERE chat_uuid = $1
		ORDER BY created_at ASC
		LIMIT 50
	`, chatUUID)

	if err != nil {
		log.Printf("Ошибка загрузки истории: %v", err)
	} else {
		defer rows.Close()
		//var history []WMessage
		//for rows.Next() {
		//	var m WMessage
		//	var senderUUID uuid.UUID
		//	if err := rows.Scan(&m.UUID, &senderUUID, &m.SenderName, &m.Content, &m.CreatedAt, &m.IsRead); err != nil {
		//		continue
		//	}
		//	m.SenderUUID = senderUUID.String()
		//	m.ChatUUID = chatUUIDStr
		//	history = append(history, m)
		//}
		//
		//for _, msg := range history {
		//	client.send <- msg
		//}
	}

	go client.writePump()
	go client.readPump()
}

func saveMessageToDB(msg WMessage) {
	go func() {
		senderUUID, err := uuid.Parse(msg.SenderUUID)
		if err != nil {
			log.Printf("Ошибка парсинга sender_uuid: %v", err)
			return
		}

		chatUUID, err := uuid.Parse(msg.ChatUUID)
		if err != nil {
			log.Printf("Некорректный chat_uuid: %v", err)
			return
		}

		msgUUID, _ := uuid.Parse(msg.UUID)

		senderName := msg.SenderName
		if senderName == "" || senderName == "аноним" {
			var name, surname string

			err = database.DB.QueryRow(`
			SELECT COALESCE(name, ''), COALESCE(surname, '') FROM users WHERE uuid = $1
		`, senderUUID).Scan(&name, &surname)
			if err == nil {
				if name != "" && surname != "" {
					senderName = name + " " + surname
				} else if name != "" {
					senderName = name
				} else if surname != "" {
					senderName = surname
				} else {
					senderName = "пользователь"
				}
			} else {
				senderName = "пользователь"
			}
		}

		_, err = database.DB.Exec(`
			INSERT INTO messages (uuid, chat_uuid, sender_uuid, sender_name, content, created_at, is_read)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (uuid) DO NOTHING
		`, msgUUID, chatUUID, senderUUID, senderName, msg.Content, msg.CreatedAt, msg.IsRead)

		if err != nil {
			log.Printf("Ошибка сохранения сообщения в БД: %v", err)
			log.Printf("Данные: content='%s', sender_name='%s', sender_uuid='%s'",
				msg.Content, senderName, msg.SenderUUID)
		} else {
			log.Printf("✓ Сообщение сохранено: '%s' от '%s'", msg.Content, senderName)
		}
	}()
}

func (h *Hub) getUserName(userUUID uuid.UUID) (string, bool) {
	if name, ok := h.userNames.Load(userUUID); ok {
		return name.(string), true
	}

	var name, surname string
	err := database.DB.QueryRow(`
		SELECT COALESCE(name, ''), COALESCE(surname, '') FROM users WHERE uuid = $1
	`, userUUID).Scan(&name, &surname)
	if err != nil {
		h.userNames.Store(userUUID, "пользователь")
		return "пользователь", true
	}
	var fullName string
	if name != "" && surname != "" {
		fullName = name + " " + surname
	} else if name != "" {
		fullName = name
	} else if surname != "" {
		fullName = surname
	} else {
		fullName = "пользователь"
	}

	h.userNames.Store(userUUID, fullName)
	return fullName, true
}
