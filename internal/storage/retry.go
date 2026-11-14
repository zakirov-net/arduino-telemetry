package storage

import (
	"context"
	"log"
	"time"
)

// OpenWithRetry пытается открыть и пинговать БД до истечения timeout.
// Удобно, когда Postgres в Docker стартует позже клиента.
func OpenWithRetry(dsn string, timeout time.Duration) (*Store, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		store, err := Open(dsn)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			perr := store.Ping(ctx)
			cancel()
			if perr == nil {
				log.Println("postgres connected")
				return store, nil
			}
			store.Close()
			err = perr
		}
		lastErr = err
		if time.Now().After(deadline) {
			return nil, lastErr
		}
		log.Printf("waiting for postgres: %v", err)
		time.Sleep(2 * time.Second)
	}
}
