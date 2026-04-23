package database

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func Connect(cfg struct{ URL string }, maxRetries int, retryInterval time.Duration) (*sql.DB, error) {
	db, err := sql.Open("pgx", cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	for i := 0; i < maxRetries; i++ {
		if err := db.Ping(); err == nil {
			log.Printf("database connected successfully")
			return db, nil
		}
		log.Printf("database not ready, retrying in %v (attempt %d/%d)", retryInterval, i+1, maxRetries)
		time.Sleep(retryInterval)
	}

	db.Close()
	return nil, fmt.Errorf("failed to connect to database after %d retries", maxRetries)
}
