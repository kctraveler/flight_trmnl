package database

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// DB holds the database connection and provides access to repositories
type DB struct {
	db *sql.DB
}

// DB returns the underlying *sql.DB connection for use by repositories
func (d *DB) DB() *sql.DB {
	return d.db
}

// AircraftRepository returns a new AircraftRepository instance
func (d *DB) AircraftRepository() AircraftRepository {
	return NewAircraftRepository(d.db)
}

// BeastMessageRepository returns a new BeastMessageRepository instance
func (d *DB) BeastMessageRepository() BeastMessageRepository {
	return NewBeastMessageRepository(d.db)
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
// Keeping schema with database.go instead of repository as it is a database level concern.
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
