package main

import (
	"chat-app/config"
	"chat-app/database"
	_ "chat-app/database"
	"chat-app/handlers"
	"chat-app/internal/redis"
	"chat-app/middleware"
	"chat-app/ws"
	"context"
	_ "database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

func main() {
	// в самом начале подключаем бд и миграции
	if err := godotenv.Load(); err != nil {
		log.Printf("Error loading .env file")
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	// Устанавливаем секрет для WebSocket
	ws.SetJWTSecret([]byte(cfg.JWT.Secret)) // <-- ДОБАВИТЬ ЭТУ СТРОКУ

	dsn := os.Getenv("GOOSE_DBSTRING")
	if dsn == "" {
		log.Fatal("DATABASE_URL не задан в .env или окружении")
	}

	db, err := goose.OpenDBWithDriver("postgres", dsn)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	database.DB = db

	//if err := goose.Up(db, "migrations"); err != nil {
	//	log.Fatal("Миграция упала:", err)
	//}

	log.Println("Миграции успешно применены!")

	redis.Init() // инициализируем редис до Hub!

	go ws.HubInstance.Run() // запускаем Hub

	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	authHandler := handlers.NewAuthHandler([]byte(cfg.JWT.Secret))

	public := r.Group("/api/v1")
	{
		public.POST("/register", authHandler.Register)
		public.POST("/login", authHandler.Login)
	}

	protected := r.Group("/api/v1")
	//protected.Use(middleware.RateLimiter())
	protected.Use(middleware.AuthMiddleware([]byte(cfg.JWT.Secret)))
	{
		protected.POST("/refresh-token", authHandler.RefreshToken)
		protected.POST("/logout", authHandler.Logout)
		protected.GET("/profile", handlers.GetUserProfile)

		protected.POST("/message", authHandler.SendMessage)

		protected.POST("/chats/direct", handlers.CreateDirectChat)
		protected.POST("/chats/group", handlers.CreateGroupChat)
		protected.GET("/chats", handlers.GetUserChats)
		protected.GET("/chats/:chat_uuid/messages", handlers.GetChatMessages)
		protected.GET("/users/search", handlers.SearchUsers)
		protected.GET("/chats/:chat_uuid/read", handlers.MarkChatAsRead)
	}

	// веб сокет, для фронта
	r.GET("/ws/chat/:chat_uuid", ws.HandleChat)

	serverAddr := cfg.Server.Host + ":" + cfg.Server.Port
	log.Printf("Server starting on %s", serverAddr)

	srv := &http.Server{
		Addr:         serverAddr,
		Handler:      r,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("Server failed to start:", err)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	log.Println("Server shutting down")
}
