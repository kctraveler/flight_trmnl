package database

import (
	"database/sql"
	"fmt"

	"flight_trmnl/internal/models"
)

type BeastMessageRepository interface {
	InsertBatch(msgs []*models.BeastMessage) error
}

type beastMessageRepository struct {
	db *sql.DB
}

func NewBeastMessageRepository(db *sql.DB) BeastMessageRepository {
	return &beastMessageRepository{db: db}
}

// InsertBatch inserts one or more Beast messages in a single transaction
// Batching is preferred over individual inserts, especially on Raspberry Pi with SD card storage.
func (r *beastMessageRepository) InsertBatch(msgs []*models.BeastMessage) error {
	if len(msgs) == 0 {
		return nil
	}

	tx, err := r.db.Begin()
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
