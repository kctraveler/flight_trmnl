package database

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"

	"flight_trmnl/internal/models"
)

type AircraftRepository interface {
	InsertBatch(aircraft []*models.Aircraft) error
	IsTablePopulated() (bool, error)
	LoadFromMultipleCSV(csvPaths []string, batchSize int) error
}

type aircraftRepository struct {
	db *sql.DB
}

func NewAircraftRepository(db *sql.DB) AircraftRepository {
	return &aircraftRepository{db: db}
}

// InsertBatch inserts one or more aircraft records in a single transaction
func (r *aircraftRepository) InsertBatch(aircraft []*models.Aircraft) error {
	if len(aircraft) == 0 {
		return nil
	}

	tx, err := r.db.Begin()
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

func (r *aircraftRepository) IsTablePopulated() (bool, error) {
	var ignored int
	err := r.db.QueryRow("SELECT 1 FROM aircraft LIMIT 1").Scan(&ignored)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to check aircraft table: %w", err)
	}
	return true, nil
}

// LoadFromMultipleCSV loads aircraft data from multiple CSV files into the database.
// File was split so that it could be uploaded to GitHub without hitting the 100MB size limit.
func (r *aircraftRepository) LoadFromMultipleCSV(csvPaths []string, batchSize int) error {
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

		for {
			record, err := reader.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("failed to read CSV record from %s: %w", csvPath, err)
			}

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
				if err := r.InsertBatch(batch); err != nil {
					return fmt.Errorf("failed to insert batch: %w", err)
				}
				batch = batch[:0] // Reset slice but keep capacity
			}
		}
	}

	// Insert remaining records
	if len(batch) > 0 {
		if err := r.InsertBatch(batch); err != nil {
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
