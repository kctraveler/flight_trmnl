package models

import (
	"fmt"
)

// Beast format constants
const (
	// BeastStartByte is the message start marker and escape character (0x1A, ASCII SUB)
	BeastStartByte byte = 0x1A

	// Beast message type indicators
	BeastTypeModeAC     byte = 0x31 // '1' - Mode A/C message (2 bytes of data)
	BeastTypeModeSShort byte = 0x32 // '2' - Mode S short message (7 bytes of data)
	BeastTypeModeSLong  byte = 0x33 // '3' - Mode S long message (14 bytes of data)

	// Beast message structure lengths
	BeastHeaderLen    = 2 // Start byte + type byte
	BeastTimestampLen = 6 // 48-bit timestamp in big-endian format
	BeastSignalLen    = 1 // Signal level byte

	// Message data lengths by type
	BeastDataLenModeAC     = 2  // Mode A/C: 2 bytes (16 bits)
	BeastDataLenModeSShort = 7  // Mode S short: 7 bytes (56 bits)
	BeastDataLenModeSLong  = 14 // Mode S long: 14 bytes (112 bits)

	// Total message lengths by type (header + timestamp + signal + data)
	BeastTotalLenModeAC     = BeastHeaderLen + BeastTimestampLen + BeastSignalLen + BeastDataLenModeAC     // 11 bytes
	BeastTotalLenModeSShort = BeastHeaderLen + BeastTimestampLen + BeastSignalLen + BeastDataLenModeSShort // 16 bytes
	BeastTotalLenModeSLong  = BeastHeaderLen + BeastTimestampLen + BeastSignalLen + BeastDataLenModeSLong  // 23 bytes

	// Minimum message length (Mode A/C is the shortest)
	BeastMinMessageLen = min(BeastTotalLenModeAC, min(BeastTotalLenModeSShort, BeastTotalLenModeSLong))
)

// GetBeastDataLen returns the message data length for a given Beast type byte
func GetBeastDataLen(typeByte byte) (int, error) {
	switch typeByte {
	case BeastTypeModeAC:
		return BeastDataLenModeAC, nil
	case BeastTypeModeSShort:
		return BeastDataLenModeSShort, nil
	case BeastTypeModeSLong:
		return BeastDataLenModeSLong, nil
	default:
		return 0, fmt.Errorf("unknown beast message type: %02x", typeByte)
	}
}

// GetBeastTotalLen returns the total message length (including header) for a given Beast type byte
func GetBeastTotalLen(typeByte byte) (int, error) {
	switch typeByte {
	case BeastTypeModeAC:
		return BeastTotalLenModeAC, nil
	case BeastTypeModeSShort:
		return BeastTotalLenModeSShort, nil
	case BeastTypeModeSLong:
		return BeastTotalLenModeSLong, nil
	default:
		return 0, fmt.Errorf("unknown beast message type: %02x", typeByte)
	}
}

// IsModeS returns true if the type byte represents a Mode S message (short or long)
func IsModeS(typeByte byte) bool {
	return typeByte == BeastTypeModeSShort || typeByte == BeastTypeModeSLong
}
