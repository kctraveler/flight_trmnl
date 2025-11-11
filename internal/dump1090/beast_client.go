package dump1090

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"flight_trmnl/internal/models"
)

// BeastClient streams Beast format messages from dump1090
type BeastClient struct {
	conn         net.Conn
	reader       *bufio.Reader
	addr         string
	maxRetries   int
	retryBackoff time.Duration
}

func NewBeastClient(addr string) *BeastClient {
	return &BeastClient{
		addr:         addr,
		maxRetries:   -1, // -1 means infinite retries
		retryBackoff: 1 * time.Second,
	}
}

// connect establishes a TCP connection to dump1090
func (c *BeastClient) connect(ctx context.Context) error {
	dialer := net.Dialer{
		Timeout: 5 * time.Second,
	}

	conn, err := dialer.DialContext(ctx, "tcp", c.addr)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", c.addr, err)
	}

	c.conn = conn
	c.reader = bufio.NewReader(conn)
	return nil
}

func (c *BeastClient) StreamMessages(ctx context.Context, messageChan chan<- *models.BeastMessage) error {
	retryCount := 0
	backoff := c.retryBackoff

	for {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Connect if not connected
		if c.conn == nil {
			if err := c.connect(ctx); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				// Connection failed, retry with exponential backoff
				retryCount++
				if c.maxRetries > 0 && retryCount > c.maxRetries {
					return fmt.Errorf("max retries (%d) exceeded", c.maxRetries)
				}
				slog.Warn("Failed to connect to Beast server", "addr", c.addr, "retry", retryCount, "error", err)
				time.Sleep(backoff)
				// Exponential backoff: 1s, 2s, 4s, 8s, max 30s
				backoff = backoff * 2
				if backoff > 30*time.Second {
					backoff = 30 * time.Second
				}
				continue
			}
			// Connection successful, reset retry state
			retryCount = 0
			backoff = c.retryBackoff
			slog.Info("Connected to Beast server", "addr", c.addr)
		}

		// Read messages in a loop
		err := c.readMessages(ctx, messageChan)
		if err != nil {
			// Connection error, close and reconnect
			slog.Warn("Connection error, reconnecting", "error", err)
			c.closeConnection()
			// Don't return, just continue to reconnect
			continue
		}

		// If we get here, context was cancelled
		return ctx.Err()
	}
}

// readByteWithEscape reads a byte, handling BeastStartByte escape sequences
// If BeastStartByte is encountered, it reads the next byte. If that's also BeastStartByte, it's an escaped byte.
// If it's not BeastStartByte, we're out of sync (unexpected new message start).
func (c *BeastClient) readByteWithEscape() (byte, error) {
	b, err := c.reader.ReadByte()
	if err != nil {
		return 0, err
	}

	if b == models.BeastStartByte {
		// Check if this is an escape sequence
		next, err := c.reader.Peek(1)
		if err != nil {
			return 0, err
		}
		if next[0] == models.BeastStartByte {
			// Escaped BeastStartByte, consume it
			c.reader.ReadByte()
			return models.BeastStartByte, nil
		}
		return 0, fmt.Errorf("unexpected %02x in data (possible sync loss)", models.BeastStartByte)
	}

	return b, nil
}

// readBytesWithEscape reads n bytes, handling BeastStartByte escape sequences
func (c *BeastClient) readBytesWithEscape(n int) ([]byte, error) {
	data := make([]byte, 0, n)
	for len(data) < n {
		b, err := c.readByteWithEscape()
		if err != nil {
			return nil, err
		}
		data = append(data, b)
	}
	return data, nil
}

func (c *BeastClient) readMessages(ctx context.Context, messageChan chan<- *models.BeastMessage) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Set read deadline
			if err := c.conn.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
				return fmt.Errorf("failed to set read deadline: %w", err)
			}

			startByte, err := c.reader.ReadByte()
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue // Timeout is OK, just retry
				}
				if err == io.EOF {
					return fmt.Errorf("connection closed")
				}
				return fmt.Errorf("failed to read start byte: %w", err)
			}

			if startByte != models.BeastStartByte {
				slog.Debug("Skipping byte, not a message start", "byte", startByte)
				continue
			}

			// Read type byte (should not be escaped, but handle it just in case)
			typeByte, err := c.reader.ReadByte()
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				if err == io.EOF {
					return fmt.Errorf("connection closed")
				}
				return fmt.Errorf("failed to read type byte: %w", err)
			}

			// Handle escape sequence for type byte (unlikely but possible)
			if typeByte == models.BeastStartByte {
				next, err := c.reader.Peek(1)
				if err != nil {
					if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
						continue
					}
					return fmt.Errorf("failed to peek after %02x in type: %w", models.BeastStartByte, err)
				}
				if next[0] == models.BeastStartByte {
					// Escaped BeastStartByte, consume it
					c.reader.ReadByte()
					typeByte = models.BeastStartByte
				} else {
					// New message start, continue
					continue
				}
			}

			// Determine message length based on type
			messageDataLen, err := models.GetBeastDataLen(typeByte)
			if err != nil {
				slog.Debug("Unknown message type", "type", typeByte, "error", err)
				continue
			}

			// Read timestamp
			timestampBytes, err := c.readBytesWithEscape(models.BeastTimestampLen)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				if err == io.EOF {
					return fmt.Errorf("connection closed")
				}
				return fmt.Errorf("failed to read timestamp: %w", err)
			}

			// Read signal level
			signalBytes, err := c.readBytesWithEscape(models.BeastSignalLen)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				if err == io.EOF {
					return fmt.Errorf("connection closed")
				}
				return fmt.Errorf("failed to read signal: %w", err)
			}

			// Read message data
			messageBytes, err := c.readBytesWithEscape(messageDataLen)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue
				}
				if err == io.EOF {
					return fmt.Errorf("connection closed")
				}
				return fmt.Errorf("failed to read message data: %w", err)
			}

			expectedTotalLen := models.BeastHeaderLen + models.BeastTimestampLen + models.BeastSignalLen + messageDataLen
			fullMessage := make([]byte, 0, expectedTotalLen)
			fullMessage = append(fullMessage, models.BeastStartByte, typeByte)
			fullMessage = append(fullMessage, timestampBytes...)
			fullMessage = append(fullMessage, signalBytes...)
			fullMessage = append(fullMessage, messageBytes...)

			beastMsg, err := models.ParseBeastMessage(fullMessage)
			if err != nil {
				// Log but continue
				slog.Debug("Failed to parse Beast message", "error", err)
				continue
			}

			select {
			case messageChan <- beastMsg:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// closeConnection closes the current connection
func (c *BeastClient) closeConnection() {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
		c.reader = nil
	}
}

// Close closes the connection
func (c *BeastClient) Close() error {
	c.closeConnection()
	return nil
}
