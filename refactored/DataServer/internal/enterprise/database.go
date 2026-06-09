package enterprise

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	_ "github.com/lib/pq"           // PostgreSQL driver
	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

// DatabaseConfig holds database configuration
type DatabaseConfig struct {
	Driver          string // "postgres" or "sqlite3"
	DSN             string // Data Source Name
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// DB wraps sql.DB with enterprise features
type DB struct {
	*sql.DB
	driver string
	mu     sync.RWMutex
}

var (
	defaultDB *DB
	once      sync.Once
)

// DefaultDatabaseConfig returns a default database configuration
func DefaultDatabaseConfig() *DatabaseConfig {
	return &DatabaseConfig{
		Driver:          "sqlite3",
		DSN:             "data/velox.db",
		MaxOpenConns:    50,
		MaxIdleConns:    10,
		ConnMaxLifetime: 30 * time.Minute,
		ConnMaxIdleTime: 5 * time.Minute,
	}
}

// NewDatabase creates a new database connection with enterprise features
func NewDatabase(cfg *DatabaseConfig) (*DB, error) {
	db, err := sql.Open(cfg.Driver, cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Connection pooling (enterprise feature)
	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{DB: db, driver: cfg.Driver}, nil
}

// InitDatabase initializes the default database
func InitDatabase(cfg *DatabaseConfig) error {
	var initErr error
	once.Do(func() {
		defaultDB, initErr = NewDatabase(cfg)
	})
	return initErr
}

// GetDB returns the default database instance
func GetDB() *DB {
	return defaultDB
}

// Driver returns the database driver name
func (db *DB) Driver() string {
	return db.driver
}

// IsPostgres returns true if using PostgreSQL
func (db *DB) IsPostgres() bool {
	return db.driver == "postgres"
}

// IsSQLite returns true if using SQLite
func (db *DB) IsSQLite() bool {
	return db.driver == "sqlite3"
}

// WithRetry executes a function with retry logic for transient errors
func (db *DB) WithRetry(ctx context.Context, maxRetries int, fn func(ctx context.Context) error) error {
	var lastErr error
	backoff := 100 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		err := fn(ctx)
		if err == nil {
			return nil
		}

		lastErr = err
		log.Printf("Database operation failed (attempt %d/%d): %v", i+1, maxRetries, err)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
			backoff *= 2
			if backoff > 5*time.Second {
				backoff = 5 * time.Second
			}
		}
	}

	return fmt.Errorf("operation failed after %d retries: %w", maxRetries, lastErr)
}

// Transaction executes a function within a transaction
func (db *DB) Transaction(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()

	if err := fn(tx); err != nil {
		if rbErr := tx.Rollback(); rbErr != nil {
			log.Printf("Failed to rollback transaction: %v", rbErr)
		}
		return err
	}

	return tx.Commit()
}

// HealthCheck performs a health check on the database
func (db *DB) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return db.PingContext(ctx)
}

// Stats returns database connection statistics
func (db *DB) Stats() *sql.DBStats {
	stats := db.DB.Stats()
	return &stats
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}
