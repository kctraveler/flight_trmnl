package database

import (
	"os"
	"testing"
	"time"

	"flight_trmnl/internal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *DB {
	// Create a temporary database file
	tmpFile := "/tmp/test_adsb_" + t.Name() + ".db"
	// Clean up any existing test database
	os.Remove(tmpFile)

	db, err := New(tmpFile)
	require.NoError(t, err)
	require.NotNil(t, db)

	return db
}

func cleanupTestDB(t *testing.T, db *DB) {
	if db != nil {
		err := db.Close()
		assert.NoError(t, err)
	}
	// Clean up test database file
	tmpFile := "/tmp/test_adsb_" + t.Name() + ".db"
	os.Remove(tmpFile)
}

func TestNew(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	// Verify database was created
	assert.NotNil(t, db)
}

func TestInsertBeastMessagesBatch(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	repo := db.BeastMessageRepository()

	msgs := []*models.BeastMessage{
		{
			Timestamp:   time.Now(),
			SignalLevel: 128,
			Message:     []byte{0x8D, 0x48, 0x40, 0xD6, 0x20, 0x2C, 0xC3, 0x71, 0xC2, 0xD7, 0x20, 0x00, 0x00, 0x00},
			ICAO:        "484040",
			MessageType: "extended_squitter",
		},
		{
			Timestamp:   time.Now().Add(time.Second),
			SignalLevel: 129,
			Message:     []byte{0x8D, 0x48, 0x41, 0xD6, 0x20, 0x2C, 0xC3, 0x71, 0xC2, 0xD7, 0x20, 0x00, 0x00, 0x01},
			ICAO:        "484041",
			MessageType: "surveillance",
		},
	}

	err := repo.InsertBatch(msgs)
	assert.NoError(t, err)
}

func TestInsertBeastMessagesBatch_Empty(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	repo := db.BeastMessageRepository()

	// Empty batch should not error
	err := repo.InsertBatch([]*models.BeastMessage{})
	assert.NoError(t, err)
}

func TestInsertBeastMessagesBatch_Duplicates(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)

	repo := db.BeastMessageRepository()

	msg := &models.BeastMessage{
		Timestamp:   time.Now(),
		SignalLevel: 128,
		Message:     []byte{0x8D, 0x48, 0x40, 0xD6, 0x20, 0x2C, 0xC3, 0x71, 0xC2, 0xD7, 0x20, 0x00, 0x00, 0x00},
		ICAO:        "484040",
		MessageType: "extended_squitter",
	}

	msgs := []*models.BeastMessage{msg, msg} // Same message twice

	// Should not error, duplicates are ignored
	err := repo.InsertBatch(msgs)
	assert.NoError(t, err)
}
