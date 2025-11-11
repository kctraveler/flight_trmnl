package models

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"
)

// BeastMessage represents a Mode S message in Beast format
type BeastMessage struct {
	Timestamp       time.Time
	SignalLevel     uint8
	Message         []byte // Variable length: BeastDataLenModeAC (Mode A/C), BeastDataLenModeSShort (Mode S short), or BeastDataLenModeSLong (Mode S long)
	MessageTypeCode byte   // Beast message type: BeastTypeModeAC, BeastTypeModeSShort, or BeastTypeModeSLong
	ICAO            string // Extracted ICAO address (first 3 bytes of message, for Mode S only)
	MessageType     string // Type of message (position, identity, etc.)
}

// ParseBeastMessage parses a Beast format message
// Beast format: BeastStartByte [Type] [BeastTimestampLen-byte timestamp] [BeastSignalLen-byte signal] [variable-length message]
// The 'Type' field indicates the message kind and determines message length:
//   - BeastTypeModeAC (0x31): Mode A/C, expects BeastDataLenModeAC (2 bytes)
//   - BeastTypeModeSShort (0x32): Mode S short, expects BeastDataLenModeSShort (7 bytes)
//   - BeastTypeModeSLong (0x33): Mode S long, expects BeastDataLenModeSLong (14 bytes)
func ParseBeastMessage(data []byte) (*BeastMessage, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("beast message data must not be empty")
	}

	// Check for Beast format header
	if data[0] != BeastStartByte {
		return nil, fmt.Errorf("invalid beast header start byte: %02x", data[0])
	}

	// Check message type
	typeByte := data[1]
	expectedDataLen, err := GetBeastDataLen(typeByte)
	if err != nil {
		return nil, err
	}

	// Check message size for type is correct
	expectedTotalLen := BeastHeaderLen + BeastTimestampLen + BeastSignalLen + expectedDataLen
	if len(data) != expectedTotalLen {
		return nil, fmt.Errorf("beast message length mismatch: got %d bytes, expected %d for type %02x", len(data), expectedTotalLen, typeByte)
	}

	// Extract timestamp (BeastTimestampLen bytes, big-endian, 12 MHz clock ticks)
	// The timestamp is a 48-bit value stored in 6 bytes as big-endian
	timestampOffset := BeastHeaderLen
	timestampBytes := data[timestampOffset : timestampOffset+BeastTimestampLen]

	// Convert 48-bit big-endian to int64 by padding with zeros and using binary.BigEndian
	// We create an 8-byte buffer with 2 leading zeros, then read as uint64
	var timestampBuf [8]byte
	copy(timestampBuf[2:], timestampBytes)
	timestampTicks := int64(binary.BigEndian.Uint64(timestampBuf[:]))

	// Convert from 12 MHz clock ticks to seconds
	// Note: The timestamp is relative to the start of the sample block, not absolute Unix time
	// For now, we'll use the current time as a reference, but ideally we'd track a base time
	// Since we don't have the base time, we'll use current time as approximation
	// In practice, you might want to track the first timestamp and use it as a base
	timestampSeconds := float64(timestampTicks) / 12000000.0
	timestamp := time.Now().Add(-time.Duration(timestampSeconds) * time.Second)

	// Extract signal level (BeastSignalLen byte)
	signalOffset := BeastHeaderLen + BeastTimestampLen
	signalLevel := data[signalOffset]

	// Extract message data
	messageStart := BeastHeaderLen + BeastTimestampLen + BeastSignalLen
	messageEnd := messageStart + expectedDataLen
	message := make([]byte, expectedDataLen)
	copy(message, data[messageStart:messageEnd])

	// Extract ICAO address (only for Mode S messages, not Mode A/C)
	var icao string
	var messageType string
	if IsModeS(typeByte) {
		// Mode S message - extract ICAO and determine message type
		icao = extractICAO(message)
		messageType = determineMessageType(message)
	} else {
		// Mode A/C message
		icao = ""
		messageType = "mode_ac"
	}

	return &BeastMessage{
		Timestamp:       timestamp,
		SignalLevel:     signalLevel,
		Message:         message,
		MessageTypeCode: typeByte,
		ICAO:            icao,
		MessageType:     messageType,
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
	case 16, 17, 18, 19:
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
