# Beast Mode Output Format (Port 30005)

## Overview
Port 30005 is the default TCP port for Beast mode binary output from dump1090. This format is used to transmit ADS-B (Automatic Dependent Surveillance-Broadcast) messages received from aircraft via software-defined radio.

## Message Structure

Each Beast message follows this binary format:

```
[0x1A] [Type] [Timestamp (6 bytes)] [Signal (1 byte)] [Message Data (variable)]
```

### Message Components

1. **Start Byte (0x1A)**: Every message starts with `0x1A` (ASCII SUB character). This also serves as an escape character.

2. **Type Indicator (1 byte)**:
   - `'1'` (0x31): Mode A/C message - 2 bytes of data
   - `'2'` (0x32): Mode S short message - 7 bytes of data  
   - `'3'` (0x33): Mode S long message - 14 bytes of data

3. **Timestamp (6 bytes, big-endian)**:
   - 48-bit timestamp in big-endian format
   - Represents time in 12 MHz clock ticks (for dump1090)
   - Format: `(byte0 << 40) | (byte1 << 32) | (byte2 << 24) | (byte3 << 16) | (byte4 << 8) | byte5`
   - To convert to seconds: `timestamp / 12,000,000`
   - The timestamp represents when the message was received relative to the start of the sample block

4. **Signal Level (1 byte)**:
   - Signal strength indicator (0-255)
   - Represents `sqrt(signalLevel) * 255` where signalLevel is the actual signal strength
   - To convert back: `signalLevel = (signal / 255.0)^2`

5. **Message Data (variable length)**:
   - Mode A/C: 2 bytes (16 bits)
   - Mode S short: 7 bytes (56 bits)
   - Mode S long: 14 bytes (112 bits)
   - Contains the actual ADS-B message **bits packed into bytes**
   - **Bit ordering**: Bits are packed MSB-first within each byte
   - **Bit numbering**: Bit 1 is the MSB of the first byte, bit 2 is the next bit, etc.
   - The bytes are transmitted as-is; you need to extract individual bits using bit operations
   - Example: To get bit 1 (DF field), extract the MSB (bit 7) of byte 0

### Escape Character Handling

The byte `0x1A` is used as an escape character. If any byte in the message (timestamp, signal, or data) equals `0x1A`, it is **doubled** (written twice) to distinguish it from the message start marker.

Example: If a timestamp byte is `0x1A`, it will be written as `0x1A 0x1A`.

## Message Examples

### Mode S Long Message (14 bytes):
```
0x1A 0x33 [6-byte timestamp] [1-byte signal] [14-byte ADS-B message]
```

### Mode S Short Message (7 bytes):
```
0x1A 0x32 [6-byte timestamp] [1-byte signal] [7-byte ADS-B message]
```

### Mode A/C Message (2 bytes):
```
0x1A 0x31 [6-byte timestamp] [1-byte signal] [2-byte Mode A/C data]
```

## Heartbeat Messages

Periodically, heartbeat messages are sent:
```
0x1A 0x31 0x00 0x00 0x00 0x00 0x00 0x00 0x00 0x00 0x00
```
This is a Mode A/C message with all zeros, used to keep the connection alive.

## Implementation Details

From the code (`net_io.c`, function `writeBeastMessage`):

- The timestamp is 48 bits (6 bytes) in big-endian format
- Signal level is calculated as: `round(sqrt(signalLevel) * 255)`, clamped to 1-255
- All bytes are checked for `0x1A` and escaped if found
- Maximum buffer size: `2 + 2 * (7 + msgLen)` bytes (accounts for potential escaping)

## Parsing Algorithm

To parse Beast messages:

1. Look for `0x1A` as the start marker
2. Read the type byte (`'1'`, `'2'`, or `'3'`)
3. Read 6 bytes for timestamp (handle `0x1A` escaping)
4. Read 1 byte for signal level (handle `0x1A` escaping)
5. Read message data based on type:
   - Type `'1'`: 2 bytes
   - Type `'2'`: 7 bytes
   - Type `'3'`: 14 bytes
   - Handle `0x1A` escaping in all data bytes

## Reference Code

See `tools/replay-beast.py` for a Python implementation that parses this format.

## Connection Details

- **Default Port**: 30005 (configurable via `--net-bo-port`)
- **Protocol**: TCP
- **Default Mode**: "Cooked" mode (messages may be corrected/filtered)
- **Alternative Modes**: 
  - Verbatim mode: forwards messages exactly as received (uses `mm->verbatim` instead of `mm->msg`)
  - Local-only mode: only local (non-remote) messages

### Output Modes Explained

**Cooked Mode** (default):
- Messages may have been error-corrected using CRC
- Filters out:
  - Messages with 2+ bit corrections
  - Unreliable messages (from unreliable aircraft)
  - MLAT messages (unless `--forward-mlat` is set)
- Uses the corrected message data (`mm->msg`)

**Verbatim Mode**:
- Messages are sent exactly as received, before any error correction
- Uses the original raw message data (`mm->verbatim`)
- Still filters MLAT messages unless `--forward-mlat` is set
- Can be enabled via `--net-verbatim` or by sending Beast command `'V'`

**Local-Only Mode**:
- Only sends messages received locally (not forwarded from remote sources)
- Can be requested by sending Beast command `'L'`

## Bit Packing Details

While the Beast format transmits **bytes**, those bytes contain **bit-packed ADS-B messages**. The ADS-B specification defines messages in terms of bits, and dump1090 packs those bits into bytes for transmission.

### Bit Layout
- **Bit numbering starts at 1** (not 0), consistent with ADS-B specifications
- **MSB-first ordering**: Within each byte, the most significant bit (bit 7) comes first
- **Byte order**: Bytes are transmitted in order (byte 0, byte 1, etc.)

### Example: Extracting the DF (Downlink Format) Field
The DF field is bits 1-5 of a Mode S message:
- Bit 1 = MSB of byte 0 (bit 7 of the byte)
- Bit 2 = bit 6 of byte 0
- Bit 3 = bit 5 of byte 0
- Bit 4 = bit 4 of byte 0
- Bit 5 = bit 3 of byte 0

To extract: `df = (byte0 >> 3) & 0x1F`

### Reference Implementation
See `mode_s.h` functions `getbit()` and `getbits()` for how dump1090 extracts bits:
- `getbit(data, bitnum)`: Extracts a single bit (bitnum is 1-based)
- `getbits(data, firstbit, lastbit)`: Extracts a range of bits

## Related Files

- `net_io.c`: Contains `writeBeastMessage()` function (line 440)
- `tools/replay-beast.py`: Example parser implementation
- `mode_s.h`: Contains `getbit()` and `getbits()` functions for bit extraction (lines 94-149)
- `dump1090.h`: Defines message size constants:
  - `MODEAC_MSG_BYTES = 2` (16 bits)
  - `MODES_SHORT_MSG_BYTES = 7` (56 bits)
  - `MODES_LONG_MSG_BYTES = 14` (112 bits)

