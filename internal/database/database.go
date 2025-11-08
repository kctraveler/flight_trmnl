package database

import (
	"database/sql"
	"fmt"

	"flight_trmnl/internal/models"

	_ "github.com/mattn/go-sqlite3"
)

// Repository defines the interface for Beast message storage operations
type Repository interface {
	InsertBeastMessage(msg *models.BeastMessage) error
	InsertBeastMessagesBatch(msgs []*models.BeastMessage) error
	Close() error
}

// DB implements the Repository interface using SQLite
type DB struct {
	db *sql.DB
}

// New creates and initializes a new database connection
func New(dbPath string) (*DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Optimize SQLite for Raspberry Pi performance
	if err := optimizeSQLite(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to optimize database: %w", err)
	}

	database := &DB{db: db}

	if err := database.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return database, nil
}

// optimizeSQLite applies performance optimizations for Raspberry Pi
func optimizeSQLite(db *sql.DB) error {
	// Enable WAL mode for better concurrency (allows concurrent reads)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Increase cache size (default is 2MB, set to 64MB for better performance)
	// This uses RAM, not disk, so it's safe
	if _, err := db.Exec("PRAGMA cache_size=-64000"); err != nil {
		return fmt.Errorf("failed to set cache size: %w", err)
	}

	// Use NORMAL synchronous mode (faster than FULL, safer than OFF)
	// WAL mode makes this safer since writes go to WAL first
	if _, err := db.Exec("PRAGMA synchronous=NORMAL"); err != nil {
		return fmt.Errorf("failed to set synchronous mode: %w", err)
	}

	// Increase temp_store to use memory instead of disk for temp tables
	if _, err := db.Exec("PRAGMA temp_store=MEMORY"); err != nil {
		return fmt.Errorf("failed to set temp_store: %w", err)
	}

	// Set busy timeout to handle concurrent access better
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		return fmt.Errorf("failed to set busy timeout: %w", err)
	}

	return nil
}

// Close closes the database connection
func (d *DB) Close() error {
	return d.db.Close()
}

// initSchema creates the database schema if it doesn't exist
func (d *DB) initSchema() error {
	messagesSchema := `CREATE TABLE IF NOT EXISTS beast_messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp TIMESTAMP NOT NULL,
		icao TEXT NOT NULL,
		message_type TEXT,
		signal_level INTEGER,
		message_hex TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(icao, timestamp, message_hex)
	);`

	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_beast_messages_icao ON beast_messages(icao)`,
		`CREATE INDEX IF NOT EXISTS idx_beast_messages_timestamp ON beast_messages(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_beast_messages_icao_timestamp ON beast_messages(icao, timestamp)`,
	}

	if _, err := d.db.Exec(messagesSchema); err != nil {
		return fmt.Errorf("failed to create beast_messages table: %w", err)
	}

	for _, idx := range indexes {
		if _, err := d.db.Exec(idx); err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// InsertBeastMessage inserts a single Beast format message
func (d *DB) InsertBeastMessage(msg *models.BeastMessage) error {
	query := `INSERT OR IGNORE INTO beast_messages (
		timestamp, icao, message_type, signal_level, message_hex
	) VALUES (?, ?, ?, ?, ?)`

	_, err := d.db.Exec(query,
		msg.Timestamp,
		msg.ICAO,
		msg.MessageType,
		msg.SignalLevel,
		msg.Hex(),
	)

	return err
}

// InsertBeastMessagesBatch inserts multiple Beast messages in a single transaction
// This is much faster than individual inserts, especially on Raspberry Pi
func (d *DB) InsertBeastMessagesBatch(msgs []*models.BeastMessage) error {
	if len(msgs) == 0 {
		return nil
	}

	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO beast_messages (
		timestamp, icao, message_type, signal_level, message_hex
	) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, msg := range msgs {
		if _, err := stmt.Exec(
			msg.Timestamp,
			msg.ICAO,
			msg.MessageType,
			msg.SignalLevel,
			msg.Hex(),
		); err != nil {
			return fmt.Errorf("failed to insert message: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

