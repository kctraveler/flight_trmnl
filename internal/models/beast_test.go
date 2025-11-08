package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBeastMessage(t *testing.T) {
	tests := []struct {
		name      string
		data      []byte
		wantErr   bool
		checkFunc func(*testing.T, *BeastMessage, error)
	}{
		{
			name: "valid Beast message",
			data: []byte{
				0x1a, 0x31, // Header
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Timestamp (6 bytes)
				0x80,                   // Signal level
				0x8D, 0x48, 0x40, 0xD6, 0x20, 0x2C, 0xC3, 0x71, 0xC2, 0xD7, 0x20, 0x00, 0x00, 0x00, // Message (14 bytes)
			},
			wantErr: false,
			checkFunc: func(t *testing.T, msg *BeastMessage, err error) {
				require.NoError(t, err)
				require.NotNil(t, msg)
				assert.Equal(t, uint8(0x80), msg.SignalLevel)
				assert.NotEmpty(t, msg.ICAO)
				assert.NotEmpty(t, msg.MessageType)
				assert.Len(t, msg.Message, 14)
			},
		},
		{
			name:    "message too short",
			data:    []byte{0x1a, 0x31},
			wantErr: true,
			checkFunc: func(t *testing.T, msg *BeastMessage, err error) {
				assert.Error(t, err)
				assert.Nil(t, msg)
			},
		},
		{
			name: "invalid header",
			data: []byte{
				0x1a, 0x32, // Invalid header
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x80,
				0x8D, 0x48, 0x40, 0xD6, 0x20, 0x2C, 0xC3, 0x71, 0xC2, 0xD7, 0x20, 0x00, 0x00, 0x00,
			},
			wantErr: true,
			checkFunc: func(t *testing.T, msg *BeastMessage, err error) {
				assert.Error(t, err)
				assert.Nil(t, msg)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := ParseBeastMessage(tt.data)
			tt.checkFunc(t, msg, err)
		})
	}
}

func TestExtractICAO(t *testing.T) {
	tests := []struct {
		name     string
		message  []byte
		expected string
	}{
		{
			name:     "valid ICAO extraction",
			message:  []byte{0x8D, 0x48, 0x40, 0xD6, 0x20, 0x2C, 0xC3, 0x71, 0xC2, 0xD7, 0x20, 0x00, 0x00, 0x00},
			expected: "054840", // First 3 bytes: (0x8D & 0x07) << 16 | 0x48 << 8 | 0x40 = 0x05 << 16 | 0x48 << 8 | 0x40
		},
		{
			name:     "short message",
			message:  []byte{0x8D, 0x48},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			icao := extractICAO(tt.message)
			assert.Equal(t, tt.expected, icao)
		})
	}
}

func TestDetermineMessageType(t *testing.T) {
	tests := []struct {
		name     string
		message  []byte
		expected string
	}{
		{
			name:     "surveillance message (DF 0)",
			message:  []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			expected: "surveillance",
		},
		{
			name:     "extended squitter (DF 17)",
			message:  []byte{0x88, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			expected: "extended_squitter",
		},
		{
			name:     "empty message",
			message:  []byte{},
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msgType := determineMessageType(tt.message)
			assert.Equal(t, tt.expected, msgType)
		})
	}
}

func TestBeastMessage_Hex(t *testing.T) {
	msg := &BeastMessage{
		Message: []byte{0x8D, 0x48, 0x40, 0xD6, 0x20, 0x2C, 0xC3, 0x71, 0xC2, 0xD7, 0x20, 0x00, 0x00, 0x00},
	}

	hex := msg.Hex()
	assert.Equal(t, "8d4840d6202cc371c2d720000000", hex)
}

func TestParseBeastMessage_Timestamp(t *testing.T) {
	// Create a message with a specific timestamp
	// Timestamp: milliseconds since Unix epoch
	timestampMs := int64(1609459200000) // 2021-01-01 00:00:00 UTC

	data := []byte{
		0x1a, 0x31, // Header
		byte(timestampMs >> 40), byte(timestampMs >> 32), byte(timestampMs >> 24),
		byte(timestampMs >> 16), byte(timestampMs >> 8), byte(timestampMs), // Timestamp (6 bytes)
		0x80,                   // Signal level
		0x8D, 0x48, 0x40, 0xD6, 0x20, 0x2C, 0xC3, 0x71, 0xC2, 0xD7, 0x20, 0x00, 0x00, 0x00, // Message
	}

	msg, err := ParseBeastMessage(data)
	require.NoError(t, err)
	require.NotNil(t, msg)

	// Check that timestamp is approximately correct (within 1 second)
	expectedTime := time.Unix(timestampMs/1000, (timestampMs%1000)*1000000)
	assert.WithinDuration(t, expectedTime, msg.Timestamp, time.Second)
}

