package database

import (
	"database/sql"
	"log"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/lib/pq"
)

var DB *sql.DB

type Database struct {
	DB *sql.DB
}

//func NewDatabase(connectionString string) (*Database, error) {
//	//Open database connection
//	db, err := sql.Open("postgres", connectionString)
//	if err != nil {
//		return nil, err
//	}
//
//	// Configure connection pool
//	db.SetMaxOpenConns(25)
//	db.SetMaxIdleConns(5)
//	db.SetConnMaxLifetime(time.Minute * 5)
//
//	// Verify connection is working
//	if err := db.Ping(); err != nil {
//		return nil, err
//	}
//
//	return &Database{DB: db}, nil
//}

func NewDatabase(connectionString string) (*Database, error) {
	// Пытаемся подключиться к реальной БД
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		log.Println("БД не доступна, работаем без неё:", err)
		// Возвращаем заглушку — сервер не упадёт
		return &Database{DB: &sql.DB{}}, nil
	}

	// Настраиваем пул соединений
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Minute * 5)

	// Проверяем, что БД жива
	if err := db.Ping(); err != nil {
		log.Println("БД не отвечает, работаем без неё:", err)
		db.Close()
		return &Database{DB: &sql.DB{}}, nil
	}

	DB = db

	log.Println("Успешно подключились к PostgreSQL")
	return &Database{DB: db}, nil
}
