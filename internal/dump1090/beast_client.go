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

// readBytesWithEscape reads n bytes, handling BeastStartByte escape sequences
// If BeastStartByte is encountered, it reads the next byte. If that's also BeastStartByte, it's an escaped byte.
// If it's not BeastStartByte, we're out of sync (unexpected new message start).
func (c *BeastClient) readBytesWithEscape(n int) ([]byte, error) {
	data := make([]byte, 0, n)
	for len(data) < n {
		b, err := c.reader.ReadByte()
		if err != nil {
			return nil, err
		}

		if b == models.BeastStartByte {
			// Check if this is an escape sequence
			next, err := c.reader.Peek(1)
			if err != nil {
				return nil, err
			}
			if next[0] == models.BeastStartByte {
				// Escaped BeastStartByte, consume it
				c.reader.ReadByte()
				data = append(data, models.BeastStartByte)
			} else {
				// Unexpected new message start - sync loss
				return nil, fmt.Errorf("unexpected %02x in data (possible sync loss)", models.BeastStartByte)
			}
		} else {
			data = append(data, b)
		}
	}
	return data, nil
}

// handleReadError handles read errors, returning nil for timeouts (to retry) and errors for other cases
func (c *BeastClient) handleReadError(err error) error {
	if err == nil {
		return nil
	}
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return nil // Timeout is OK, caller will retry
	}
	if err == io.EOF {
		return fmt.Errorf("connection closed")
	}
	return err
}

func (c *BeastClient) readMessages(ctx context.Context, messageChan chan<- *models.BeastMessage) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Set read deadline
		if err := c.conn.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
			return fmt.Errorf("failed to set read deadline: %w", err)
		}

		// Read start byte
		startByte, err := c.reader.ReadByte()
		if processedErr := c.handleReadError(err); processedErr != nil {
			return fmt.Errorf("failed to read start byte: %w", processedErr)
		}
		if err != nil {
			continue // Timeout, retry
		}

		if startByte != models.BeastStartByte {
			slog.Debug("Skipping byte, not a message start", "byte", startByte)
			continue
		}

		// Read type byte (should not be escaped, but handle it just in case)
		typeByte, err := c.reader.ReadByte()
		if processedErr := c.handleReadError(err); processedErr != nil {
			return fmt.Errorf("failed to read type byte: %w", processedErr)
		}
		if err != nil {
			continue // Timeout, retry
		}

		// Handle escape sequence for type byte (unlikely but possible)
		if typeByte == models.BeastStartByte {
			next, err := c.reader.Peek(1)
			if processedErr := c.handleReadError(err); processedErr != nil {
				return fmt.Errorf("failed to peek after %02x in type: %w", models.BeastStartByte, processedErr)
			}
			if err != nil {
				continue // Timeout, retry
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

		totalLen, err := models.GetBeastTotalLen(typeByte)
		if err != nil {
			slog.Debug("Unknown message type", "type", typeByte, "error", err)
			continue
		}

		// Read remaining bytes (timestamp + signal + message data) all at once
		// We've already read 2 bytes (start + type), so read totalLen - 2
		remainingBytes, err := c.readBytesWithEscape(totalLen - models.BeastHeaderLen)
		if processedErr := c.handleReadError(err); processedErr != nil {
			return fmt.Errorf("failed to read message body: %w", processedErr)
		}
		if err != nil {
			continue // Timeout, retry
		}

		// Assemble full message
		fullMessage := make([]byte, 0, totalLen)
		fullMessage = append(fullMessage, models.BeastStartByte, typeByte)
		fullMessage = append(fullMessage, remainingBytes...)

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
