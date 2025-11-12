# Flight Terminal

A Go application for collecting and persisting ADS-B aircraft data from dump1090 on Raspberry Pi.

## Overview

Flight Terminal is a lightweight application that connects to dump1090's Beast format output and streams ADS-B messages to a local SQLite database in real-time. It's designed to run continuously on a Raspberry Pi alongside dump1090 and piaware, collecting the same raw message data that piaware sends to FlightAware.

The application streams messages at high rates (~200 messages/second at peak) and efficiently batches writes to the database for optimal performance on resource-constrained devices.

## Current Features

- **Real-time Data Collection**: Streams all ADS-B messages from dump1090 via Beast format (TCP port 30005)
- **Efficient Batching**: Groups messages into batches of 100 or flushes every 1 second, whichever comes first
- **Local Persistence**: Stores messages in SQLite with optimizations for high write rates
- **Raspberry Pi Optimized**: Designed for resource-constrained environments with performance optimizations for SD card storage
- **Connection Resilience**: Automatically reconnects to dump1090 on connection failures
- **Graceful Shutdown**: Flushes remaining messages before exiting

## Architecture

### Core Components

- **BeastClient**: Connects to dump1090's Beast format output (TCP port 30005) and streams messages in real-time with automatic reconnection
- **BeastCollector**: Collects messages from the stream and batches them for efficient database writes (100 messages or 1 second timeout)
- **Database**: SQLite storage with WAL mode, memory caching, and other optimizations for high write rates on Raspberry Pi
- **Aircraft Database**: Pre-loaded aircraft registration database for ICAO address lookup

### Data Flow

1. **BeastClient** connects to dump1090 and streams Beast format messages
2. Messages are sent through a buffered channel (1000 message capacity)
3. **BeastCollector** receives messages and batches them
4. Batches are written to SQLite in transactions for efficiency

## Usage

### Configuration

The application can be configured via a YAML config file or environment variables. See `config.yaml.example` for the full configuration format.

Key configuration options:
- `beast_addr`: Beast format address (default: `localhost:30005`)
- `db_path`: Database file path (default: `adsb_data.db`)
- `log.level`: Logging level - `debug`, `info`, `warn`, or `error` (default: `info`)
- `log.format`: Log format - `text` or `json` (default: `text`)

### Running

```bash
go build -o flight_trmnl
./flight_trmnl
```

Or with a custom config file:

```bash
./flight_trmnl -config /path/to/config.yaml
```

### Debug Mode

To see detailed message logging, set the log level to `debug` in your config:

```yaml
log:
  level: debug
  format: text
```

This will log each message as it's added to the batch, including ICAO address, message type, signal level, timestamp, and current batch size.

## Raspberry Pi Considerations

The application is optimized for Raspberry Pi environments:

- **Batch Writes**: Groups messages into batches (100 messages or 1 second) to reduce SD card write frequency
- **WAL Mode**: Uses Write-Ahead Logging for better concurrency and performance
- **Memory Caching**: 64MB cache size to reduce disk I/O
- **Connection Resilience**: Automatically reconnects to dump1090 on network interruptions
- **Buffered Channels**: 1000 message buffer to handle message rate spikes

For best performance, consider using a high-endurance SD card or USB SSD for the database, especially if running continuously for extended periods.

## Data Model

The application stores individual Beast format messages in the `beast_messages` table:

- `id`: Auto-incrementing primary key
- `timestamp`: Message timestamp (see Known Issues below)
- `icao`: Aircraft ICAO address (24-bit hex)
- `message_type`: Type of ADS-B message (Mode A/C, Mode S short, Mode S long)
- `signal_level`: Signal strength (0-255)
- `message_hex`: Raw message in hex format
- `created_at`: Database insertion timestamp

The application also maintains an `aircraft` table with aircraft registration data loaded from CSV files, keyed by ICAO address.

## Planned Features

The following features are planned for future releases:

- **Scheduled Analytics**: Periodic analysis of collected data to generate statistics, flight patterns, and aircraft activity reports
- **Report Pushing**: Automated pushing of analytics and reports to external systems via HTTPS/API endpoints
- **Database Rotation**: Automatic purging of old messages based on retention policies to manage storage space
- **Enhanced Message Parsing**: Extract additional data from message bodies, including flight call signs for better flight tracking
- **Alert System**: Detection and notification of emergency codes or other interesting events (e.g., via email)
- **Aircraft Tracking**: Tools for tracking specific aircraft over time

## Known Issues

### Timestamp Inaccuracy

The timestamp parsing in Beast messages is currently inaccurate. The Beast format provides timestamps as 48-bit values representing 12 MHz clock ticks relative to the start of a sample block, not absolute Unix timestamps. The current implementation attempts to approximate absolute time by subtracting the relative time from the current time, but this is not accurate.

**Impact**: Message timestamps stored in the database may not reflect the exact time the message was received.

**Workaround**: The `created_at` field provides the actual database insertion time, which can be used as a more reliable timestamp for when the message was processed.

**Status**: This is a known bug tracked in `TODO.MD` and requires research into the proper Beast timestamp format to fix.

## Development

The project is organized into focused subsystems:

- `internal/dump1090`: Beast format client with connection management
- `internal/database`: SQLite storage layer with repositories
- `internal/tasks`: Task implementations (currently BeastCollector)
- `internal/models`: Beast message parsing and data models
- `internal/config`: Configuration management
