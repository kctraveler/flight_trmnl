# Flight Terminal

A Go daemon for collecting and persisting ADS-B aircraft data from dump1090 on Raspberry Pi.

## Overview

Flight Terminal is a lightweight daemon that connects to dump1090's Beast format output and stores ADS-B messages in a local SQLite database. It's designed to run continuously on a Raspberry Pi alongside dump1090 and piaware, collecting the same raw message data that piaware sends to FlightAware.

## Goals

- **Data Collection**: Collect all ADS-B messages from dump1090 in real-time via Beast format. This is the same format piaware uses.
- **Local Persistence**: Store messages in SQLite for local analysis and historical tracking
- **Raspberry Pi Optimized**: Designed for resource-constrained environments with performance optimizations for SD card storage
- **Extensible Architecture**: Built with a task scheduler that supports multiple concurrent tasks running on different schedules. Will need to support scheduled data reporting, pushing of data to other systems via https, data retention rules to free storage.

## Architecture

### Core Components

- **BeastClient**: Connects to dump1090's Beast format output (TCP port 30005) and streams messages in real-time
- **BeastCollector**: Collects messages from the stream and batches them for efficient database writes
- **Database**: SQLite storage with optimizations for high write rates on Raspberry Pi
- **Scheduler**: Task scheduling system that supports multiple concurrent tasks with different intervals

### Task Scheduler

The scheduler is designed for extensibility. You can easily add new scheduled tasks that run independently:

- **Data Collection**: Runs continuously, collecting messages from dump1090
- **Future Tasks**: Data summarization, retention policies, external API pushes, health monitoring, etc.

Each task implements the `Task` interface and runs on its own schedule, allowing you to add new functionality without modifying existing code.

## Usage

### Configuration

The daemon can be configured via environment variables:

- `BEAST_ADDR`: Beast format address (default: `localhost:30005`)
- `ADSB_DB_PATH`: Database file path (default: `adsb_data.db`)
- `BATCH_SIZE`: Number of messages to batch before writing (default: `100`)
- `BATCH_TIMEOUT`: Seconds before flushing batch even if not full (default: `5`)

### Running

```bash
go build -o flight_trmnl
./flight_trmnl
```

Or with custom configuration:

```bash
BEAST_ADDR=192.168.1.100:30005 ADSB_DB_PATH=/data/adsb.db ./flight_trmnl
```

## Raspberry Pi Considerations

The daemon is optimized for Raspberry Pi environments:

- **Batch Writes**: Groups messages into batches to reduce SD card write frequency
- **WAL Mode**: Uses Write-Ahead Logging for better concurrency and performance
- **Memory Caching**: Configurable cache size to reduce disk I/O
- **Connection Resilience**: Handles network interruptions gracefully

For best performance, consider using a high-endurance SD card or USB SSD for the database.

## Data Model

The daemon stores individual Beast format messages in the `beast_messages` table:

- `timestamp`: Message timestamp from dump1090
- `icao`: Aircraft ICAO address
- `message_type`: Type of ADS-B message
- `signal_level`: Signal strength
- `message_hex`: Raw message in hex format

Messages are deduplicated using a unique constraint on `(icao, timestamp, message_hex)`.

## Development

The project is organized into focused subsystems:

- `internal/dump1090`: Beast format client
- `internal/database`: SQLite storage layer
- `internal/tasks`: Scheduled task implementations
- `internal/scheduler`: Task scheduling system
- `internal/daemon`: Daemon orchestration
