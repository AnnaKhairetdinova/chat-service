package handlers

import (
	_ "archive/zip"
	"chat-app/database"
	"chat-app/internal/models"
	"chat-app/utils"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type AuthHandler struct {
	db              *sql.DB
	jwtSecret       []byte
	tokenExpiration time.Duration
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler(jwtSecret []byte) *AuthHandler {
	return &AuthHandler{
		db:              database.DB,
		jwtSecret:       jwtSecret,
		tokenExpiration: 24 * time.Hour,
	}
}

// Register handles user registration
func (h *AuthHandler) Register(c *gin.Context) {
	var user models.UserRegister

	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid input format",
			"details": err.Error(),
		})
		return
	}

	if err := user.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var exists bool
	err := h.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`,
		user.Email).Scan(&exists)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	if exists {
		c.JSON(http.StatusConflict, gin.H{"error": "Email already registered"})
		return
	}

	var userUUID uuid.UUID
	err = h.db.QueryRow(`INSERT INTO users (name, surname, email, password_hash) VALUES ($1, $2, $3, $4) RETURNING uuid`,
		user.Name, user.Surname, user.Email, user.Password,
	).Scan(&userUUID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "User creation error"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"message":   "Registration accepted. Processing in background...",
		"user_uuid": userUUID,
	})

	go h.finalizeRegistration(userUUID, user.Password)
}

func (h *AuthHandler) finalizeRegistration(userUUID uuid.UUID, plainPassword string) {
	hashedPassword, err := utils.HashPassword(plainPassword)
	if err != nil {
		log.Printf("Failed to hash password for user %s: %v", userUUID, err)
		return
	}

	result, err := h.db.Exec(`
UPDATE users
SET password_hash = $1, updated_at = NOW()
WHERE uuid = $2`,
		hashedPassword, userUUID)

	if err != nil {
		log.Printf("Failed to finalize user %s: %v", userUUID, err)
		return
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("Failed to finalize user %s: %v", userUUID, err)
		return
	}
	if rowsAffected == 0 {
		log.Printf("User %s was already processed or not found", userUUID)
		return
	}

	log.Printf("User %s successfully registered and activated", userUUID)
}

// Login handles user authentication and JWT generation
func (h *AuthHandler) Login(c *gin.Context) {
	var login models.UserLogin
	if err := c.ShouldBindJSON(&login); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid login data"})
		return
	}

	var user models.User
	err := h.db.QueryRow(`
SELECT uuid, email, password_hash
FROM users
WHERE email = $1`,
		login.Email,
	).Scan(&user.UUID, &user.Email, &user.PasswordHash)

	if errors.Is(err, sql.ErrNoRows) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Login process failed"})
		return
	}

	if !utils.CheckPasswordHash(login.Password, user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid credentials"})
		return
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"user_uuid": user.UUID,
		"email":     user.Email,
		"iat":       now.Unix(),
		"exp":       now.Add(h.tokenExpiration).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(h.jwtSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token generation failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"access_token": tokenString,
		"expires_in":   h.tokenExpiration.Seconds(),
		"token_type":   "Bearer",
	})
}

// Login handles user authentication and JWT generation
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	userUUID, exists := c.Get("user_uuid")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"user_uuid": userUUID,
		"iat":       now.Unix(),
		"exp":       now.Add(h.tokenExpiration).Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(h.jwtSecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Token refresh failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"token":      tokenString,
		"expires_in": h.tokenExpiration.Seconds(),
		"token_type": "Bearer",
	})
}

// Logout endpoint (optional - useful for client-side cleanup)
func (h *AuthHandler) Logout(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message":     "Successfully logged out",
		"instruction": "Please remove the token from your client storage",
	})
}

func (h *AuthHandler) SendMessage(c *gin.Context) {
	var input struct {
		ChatUUID uuid.UUID `json:"chat_uuid"`
		Text     string    `json:"text"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid input format"})
		return
	}

	if input.Text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Message text is required"})
		return
	}

	senderUUID, _ := c.Get("user_uuid")
	senderUUIDStr, ok := senderUUID.(string)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user uuid"})
		return
	}

	var senderName string
	var name, surname sql.NullString
	err := h.db.QueryRow(`
		SELECT COALESCE(name, ''), COALESCE(surname, '') FROM users WHERE uuid = $1
	`, senderUUIDStr).Scan(&name, &surname)

	if err == nil {
		if name.Valid && surname.Valid && name.String != "" && surname.String != "" {
			senderName = name.String + " " + surname.String
		} else if name.Valid && name.String != "" {
			senderName = name.String
		} else if surname.Valid && surname.String != "" {
			senderName = surname.String
		} else {
			senderName = "пользователь"
		}
	} else {
		senderName = "пользователь"
	}

	var msgUUID uuid.UUID
	err = h.db.QueryRow(`
		INSERT INTO messages (chat_uuid, sender_uuid, sender_name, content, created_at, is_read)
		VALUES ($1, $2, $3, $4, NOW(), false)
		RETURNING uuid
	`, input.ChatUUID, senderUUIDStr, senderName, input.Text).Scan(&msgUUID) // <-- input.Text вместо input.Content

	if err != nil {
		log.Printf("Ошибка сохранения сообщения: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to send message"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Message sent", "uuid": msgUUID})
}

func GetUserProfile(c *gin.Context) {
	userUUIDStr := c.GetString("user_uuid")
	if userUUIDStr == "" {
		c.JSON(401, gin.H{"error": "unauthorized"})
		return
	}

	userUUID, err := uuid.Parse(userUUIDStr)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid user uuid"})
		return
	}

	var user struct {
		UUID      string    `json:"uuid"`
		Name      string    `json:"name"`
		Surname   string    `json:"surname"`
		Email     string    `json:"email"`
		CreatedAt time.Time `json:"created_at"`
	}

	err = database.DB.QueryRow(`
		SELECT uuid, name, surname, email, created_at
		FROM users
		WHERE uuid = $1
	`, userUUID).Scan(&user.UUID, &user.Name, &user.Surname, &user.Email, &user.CreatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(404, gin.H{"error": "user not found"})
			return
		}
		c.JSON(500, gin.H{"error": "database error"})
		return
	}

	c.JSON(200, gin.H{
		"user": user,
	})
}
