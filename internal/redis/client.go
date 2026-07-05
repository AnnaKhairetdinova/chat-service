package redis

import (
	"context"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

var Client *redis.Client

func Init() {
	Client = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379", // вынести в конфиг
		Password: "",
		DB:       0,
	})

	ctx, channel := context.WithTimeout(context.Background(), 5*time.Second)
	defer channel()

	if err := Client.Ping(ctx).Err(); err != nil {
		log.Fatal("Не удалось подключиться к Redis: %v", err)
	}

	log.Println("Redis подключён успешно!")
}
