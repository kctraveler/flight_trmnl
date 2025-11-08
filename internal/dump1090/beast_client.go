package dump1090

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"flight_trmnl/internal/models"
)

// BeastClient streams Beast format messages from dump1090
type BeastClient struct {
	conn   net.Conn
	reader *bufio.Reader
	addr   string
}

// NewBeastClient creates a new Beast format client
func NewBeastClient(addr string) *BeastClient {
	return &BeastClient{
		addr: addr,
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

// StreamMessages streams Beast format messages from dump1090
func (c *BeastClient) StreamMessages(ctx context.Context, messageChan chan<- *models.BeastMessage) error {
	if c.conn == nil {
		if err := c.connect(ctx); err != nil {
			return err
		}
	}

	// Read messages in a loop
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

// Close closes the connection
func (c *BeastClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

