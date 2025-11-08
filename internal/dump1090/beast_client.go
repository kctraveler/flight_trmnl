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

			// Look for Beast message header (0x1a 0x31)
			header, err := c.reader.Peek(2)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue // Timeout is OK, just retry
				}
				if err == io.EOF {
					return fmt.Errorf("connection closed")
				}
				return fmt.Errorf("failed to read header: %w", err)
			}

			// Check if we have a Beast message
			if header[0] == 0x1a && header[1] == 0x31 {
				// Read the full message (22 bytes: 2 header + 6 timestamp + 1 signal + 14 message)
				message := make([]byte, 22)
				n, err := io.ReadFull(c.reader, message)
				if err != nil {
					if err == io.EOF {
						return fmt.Errorf("connection closed")
					}
					return fmt.Errorf("failed to read message: %w", err)
				}

				if n == 22 {
					beastMsg, err := models.ParseBeastMessage(message)
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
			} else {
				// Skip one byte and try again
				c.reader.ReadByte()
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

