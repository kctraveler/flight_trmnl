package models

import (
	"encoding/hex"
	"fmt"
	"time"
)

// BeastMessage represents a Mode S message in Beast format
type BeastMessage struct {
	Timestamp   time.Time
	SignalLevel uint8
	Message     []byte // 14 bytes for 112-bit Mode S message
	ICAO        string // Extracted ICAO address (first 3 bytes of message)
	MessageType string // Type of message (position, identity, etc.)
}

// ParseBeastMessage parses a Beast format message
// Beast format: 0x1a 0x31 [6-byte timestamp] [1-byte signal] [14-byte message]
func ParseBeastMessage(data []byte) (*BeastMessage, error) {
	if len(data) < 22 {
		return nil, fmt.Errorf("beast message too short: %d bytes", len(data))
	}

	// Check for Beast format header
	if data[0] != 0x1a || data[1] != 0x31 {
		return nil, fmt.Errorf("invalid beast header: %02x %02x", data[0], data[1])
	}

	// Extract timestamp (6 bytes, milliseconds since Unix epoch)
	timestampMs := int64(data[2])<<40 | int64(data[3])<<32 | int64(data[4])<<24 |
		int64(data[5])<<16 | int64(data[6])<<8 | int64(data[7])
	timestamp := time.Unix(timestampMs/1000, (timestampMs%1000)*1000000)

	// Extract signal level (1 byte)
	signalLevel := data[8]

	// Extract Mode S message (14 bytes)
	message := make([]byte, 14)
	copy(message, data[9:23])

	// Extract ICAO address (first 3 bytes of message, but need to decode properly)
	// Mode S messages have ICAO in the first 8 bits of the first byte
	// and the remaining bits in subsequent bytes
	icao := extractICAO(message)

	// Determine message type (simplified - could be enhanced)
	messageType := determineMessageType(message)

	return &BeastMessage{
		Timestamp:   timestamp,
		SignalLevel: signalLevel,
		Message:     message,
		ICAO:        icao,
		MessageType: messageType,
	}, nil
}

// extractICAO extracts the ICAO address from a Mode S message
// ICAO is 24 bits, typically in the first 8 bits of byte 0 and all of bytes 1-2
func extractICAO(message []byte) string {
	if len(message) < 3 {
		return ""
	}
	// Mode S ICAO is in the first 3 bytes
	// Format: [DF(5) + CA(3)] [ICAO(8)] [ICAO(8)]
	// We extract the lower 24 bits which contain the ICAO
	icao24 := (uint32(message[0])&0x07)<<16 | uint32(message[1])<<8 | uint32(message[2])
	return fmt.Sprintf("%06X", icao24)
}

// determineMessageType determines the type of Mode S message
func determineMessageType(message []byte) string {
	if len(message) == 0 {
		return "unknown"
	}
	// Downlink Format (DF) is in the upper 5 bits of first byte
	df := (message[0] >> 3) & 0x1F
	switch df {
	case 0, 4, 5, 11:
		return "surveillance"
	case 16, 17, 18, 19, 20, 21:
		return "extended_squitter"
	case 20, 21:
		return "comm_b"
	default:
		return "other"
	}
}

// Hex returns the message as a hex string
func (b *BeastMessage) Hex() string {
	return hex.EncodeToString(b.Message)
}

