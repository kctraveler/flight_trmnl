package database

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"

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

	aircraftSchema := `CREATE TABLE IF NOT EXISTS aircraft (
		icao24 TEXT PRIMARY KEY,
		timestamp TEXT,
		acars TEXT,
		adsb TEXT,
		built TEXT,
		categoryDescription TEXT,
		country TEXT,
		engines TEXT,
		firstFlightDate TEXT,
		firstSeen TEXT,
		icaoAircraftClass TEXT,
		lineNumber TEXT,
		manufacturerIcao TEXT,
		manufacturerName TEXT,
		model TEXT,
		modes TEXT,
		nextReg TEXT,
		notes TEXT,
		operator TEXT,
		operatorCallsign TEXT,
		operatorIata TEXT,
		operatorIcao TEXT,
		owner TEXT,
		prevReg TEXT,
		regUntil TEXT,
		registered TEXT,
		registration TEXT,
		selCal TEXT,
		serialNumber TEXT,
		status TEXT,
		typecode TEXT,
		vdl TEXT
	);`

	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_beast_messages_icao ON beast_messages(icao)`,
		`CREATE INDEX IF NOT EXISTS idx_beast_messages_timestamp ON beast_messages(timestamp)`,
	}

	if _, err := d.db.Exec(messagesSchema); err != nil {
		return fmt.Errorf("failed to create beast_messages table: %w", err)
	}

	if _, err := d.db.Exec(aircraftSchema); err != nil {
		return fmt.Errorf("failed to create aircraft table: %w", err)
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

// Check if there is at least one entry in the aircraft table
func (d *DB) IsAircraftTablePopulated() (bool, error) {
	var ignored int
	err := d.db.QueryRow("SELECT 1 FROM aircraft LIMIT 1").Scan(&ignored)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check aircraft table: %w", err)
	}
	return true, nil
}

// InsertAircraftBatch inserts multiple aircraft records in a single transaction
func (d *DB) InsertAircraftBatch(aircraft []*models.Aircraft) error {
	if len(aircraft) == 0 {
		return nil
	}

	tx, err := d.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO aircraft (
		icao24, timestamp, acars, adsb, built, categoryDescription, country,
		engines, firstFlightDate, firstSeen, icaoAircraftClass, lineNumber,
		manufacturerIcao, manufacturerName, model, modes, nextReg, notes,
		operator, operatorCallsign, operatorIata, operatorIcao, owner,
		prevReg, regUntil, registered, registration, selCal, serialNumber,
		status, typecode, vdl
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, ac := range aircraft {
		if _, err := stmt.Exec(
			ac.ICAO24, ac.Timestamp, ac.ACARS, ac.ADSB, ac.Built,
			ac.CategoryDescription, ac.Country, ac.Engines,
			ac.FirstFlightDate, ac.FirstSeen, ac.ICAOAircraftClass,
			ac.LineNumber, ac.ManufacturerICAO, ac.ManufacturerName,
			ac.Model, ac.Modes, ac.NextReg, ac.Notes, ac.Operator,
			ac.OperatorCallsign, ac.OperatorIATA, ac.OperatorICAO,
			ac.Owner, ac.PrevReg, ac.RegUntil, ac.Registered,
			ac.Registration, ac.SelCal, ac.SerialNumber, ac.Status,
			ac.TypeCode, ac.VDL,
		); err != nil {
			return fmt.Errorf("failed to insert aircraft: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// LoadAircraftFromMultipleCSV loads aircraft data from multiple CSV files into the database.
// File was split so that it could be uploaded to GitHub without hitting the 100MB size limit.
func (d *DB) LoadAircraftFromMultipleCSV(csvPaths []string, batchSize int) error {
	var headerMap map[string]int
	var expectedFields int
	batch := make([]*models.Aircraft, 0, batchSize)

	for fileIdx, csvPath := range csvPaths {
		file, err := os.Open(csvPath)
		if err != nil {
			return fmt.Errorf("failed to open CSV file %s: %w", csvPath, err)
		}
		defer file.Close()

		reader := csv.NewReader(file)
		reader.LazyQuotes = true    // Handle malformed quotes in CSV
		reader.FieldsPerRecord = -1 // Allow variable number of fields per record

		// Read header row (process and validate for every file, but only build headerMap from first)
		header, err := reader.Read()
		if err != nil {
			return fmt.Errorf("failed to read CSV header from %s: %w", csvPath, err)
		}

		// Initialize header map on first file
		if fileIdx == 0 {
			expectedFields = len(header)
			headerMap = make(map[string]int)
			for i, h := range header {
				// Remove quotes and trim whitespace
				headerMap[strings.Trim(strings.TrimSpace(h), "'\"")] = i
			}
		}

		// Process records from this file
		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("failed to read CSV record from %s: %w", csvPath, err)
			}

			// Skip rows with wrong number of fields (malformed data)
			if len(record) != expectedFields {
				continue
			}

			// Create Aircraft struct from CSV record
			ac := &models.Aircraft{
				ICAO24:              getField(record, headerMap, "icao24"),
				Timestamp:           getField(record, headerMap, "timestamp"),
				ACARS:               getField(record, headerMap, "acars"),
				ADSB:                getField(record, headerMap, "adsb"),
				Built:               getField(record, headerMap, "built"),
				CategoryDescription: getField(record, headerMap, "categoryDescription"),
				Country:             getField(record, headerMap, "country"),
				Engines:             getField(record, headerMap, "engines"),
				FirstFlightDate:     getField(record, headerMap, "firstFlightDate"),
				FirstSeen:           getField(record, headerMap, "firstSeen"),
				ICAOAircraftClass:   getField(record, headerMap, "icaoAircraftClass"),
				LineNumber:          getField(record, headerMap, "lineNumber"),
				ManufacturerICAO:    getField(record, headerMap, "manufacturerIcao"),
				ManufacturerName:    getField(record, headerMap, "manufacturerName"),
				Model:               getField(record, headerMap, "model"),
				Modes:               getField(record, headerMap, "modes"),
				NextReg:             getField(record, headerMap, "nextReg"),
				Notes:               getField(record, headerMap, "notes"),
				Operator:            getField(record, headerMap, "operator"),
				OperatorCallsign:    getField(record, headerMap, "operatorCallsign"),
				OperatorIATA:        getField(record, headerMap, "operatorIata"),
				OperatorICAO:        getField(record, headerMap, "operatorIcao"),
				Owner:               getField(record, headerMap, "owner"),
				PrevReg:             getField(record, headerMap, "prevReg"),
				RegUntil:            getField(record, headerMap, "regUntil"),
				Registered:          getField(record, headerMap, "registered"),
				Registration:        getField(record, headerMap, "registration"),
				SelCal:              getField(record, headerMap, "selCal"),
				SerialNumber:        getField(record, headerMap, "serialNumber"),
				Status:              getField(record, headerMap, "status"),
				TypeCode:            getField(record, headerMap, "typecode"),
				VDL:                 getField(record, headerMap, "vdl"),
			}

			// Skip records without ICAO24 (invalid data)
			if ac.ICAO24 == "" {
				continue
			}

			batch = append(batch, ac)

			// Insert batch when it reaches the specified size
			if len(batch) >= batchSize {
				if err := d.InsertAircraftBatch(batch); err != nil {
					return fmt.Errorf("failed to insert batch: %w", err)
				}
				batch = batch[:0] // Reset slice but keep capacity
			}
		}
	}

	// Insert remaining records
	if len(batch) > 0 {
		if err := d.InsertAircraftBatch(batch); err != nil {
			return fmt.Errorf("failed to insert final batch: %w", err)
		}
	}

	return nil
}

// getField safely retrieves a field from a CSV record by header name
func getField(record []string, headerMap map[string]int, fieldName string) string {
	if idx, ok := headerMap[fieldName]; ok && idx < len(record) {
		return strings.Trim(strings.TrimSpace(record[idx]), "'\"")
	}
	return ""
}
