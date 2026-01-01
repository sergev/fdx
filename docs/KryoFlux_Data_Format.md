# KryoFlux Stream File Data Format

**Documentation based on KryoFlux Stream Protocol Revision 1.1**
*Based on documentation by Jean Louis-Guérin (01/12/2013)*

---

## Table of Contents

1. [Introduction & Overview](#introduction--overview)
2. [Imaging Floppy Disks](#imaging-floppy-disks)
3. [KryoFlux Clocks & Counters](#kryoflux-clocks--counters)
4. [Data Format](#data-format)
5. [Stream File Structure](#stream-file-structure)
6. [Block Header](#block-header)
7. [ISB (In Stream Buffer) Blocks](#isb-in-stream-buffer-blocks)
   - [Flux Blocks](#flux-blocks)
   - [NOP Blocks](#nop-blocks)
8. [OOB (Out Of stream Buffer) Blocks](#oob-out-of-stream-buffer-blocks)
9. [Flux Data Encoding Optimization](#flux-data-encoding-optimization)
10. [Index Timing Considerations](#index-timing-considerations)
11. [RPM Interpolation](#rpm-interpolation)
12. [Decoding Stream Files](#decoding-stream-files)
13. [KryoFlux Device Behavior](#kryoflux-device-behavior)
14. [KryoFlux Hardware Information](#kryoflux-hardware-information)
15. [Parsing the Stream File](#parsing-the-stream-file)
16. [Analysis of Index Information](#analysis-of-index-information)
17. [Terminology](#terminology)
18. [References](#references)

---

## Introduction & Overview

This document provides a comprehensive description of the Stream Files used by the DTC (Disk Tool Console) program when connected to a KryoFlux Device. A Stream File is either:

- **Produced** by the DTC program in read (imaging) mode
- **Consumed** by the DTC program in write (backup) mode

### Key Characteristics

- **Binary Format**: Stream files are stored in binary and cannot be directly displayed or edited with a text editor
- **Stream Protocol**: The file content is an exact copy of the byte Stream Protocol used between the KryoFlux device and the host system when communicating over USB
- **Optimized Design**: The protocol is optimized for communication budget and CPU budget, as the KryoFlux SoC must handle high-speed flux reversal sampling (up to ~500,000 flux reversals per second for High Density disks)
- **Hardware Specific**: Stream files are hardware-specific (to the KryoFlux device) and are not intended for long-term preservation
- **Version Dependency**: This documentation is based on Version 2.2 of DTC. The format may change in future versions

**Note**: Regular users of the KryoFlux device should not be concerned by the information presented here, which is mainly of interest to programmers who want to write tools around Stream Files.

---

## Imaging Floppy Disks

To capture everything on a floppy disk, it is necessary to sample all the flux reversals between several Index Signals. The KryoFlux device:

- Starts sampling data **before** the first Index Signal
- May sample data **after** the last Index Signal
- Captures data outside Index Signals, but this data cannot be meaningfully decoded

### Multiple Revolutions

For various reasons, especially for games, multiple revolutions of data should be captured in a constant stream:

- Stream files usually contain **more than two Index Signals**
- To correctly analyze protections used on floppy disks, **multiple revolutions are required**
- For SPS to correctly produce IPF files with the CTA analyzer, the **minimum required is five revolutions (6 indexes)**

---

## KryoFlux Clocks & Counters

The KryoFlux Device is operated from a **Master Clock (mck)**. From this master clock, two synchronous clocks are derived:

1. **Sample Clock (sck)** - used by the Sample Counter to sample Flux reversals
2. **Index Clock (ick)** - used by the Index Counter to sample Index Signals

### Clock Frequencies

The clock frequencies are defined by the KryoFlux hardware and can be queried using a device command. The **default values** are stored as 64-bit floating points:

| Abbreviation | Name | Clock Value | Formula |
|--------------|------|-------------|---------|
| **mck** | Master Clock | 48054857.14285714 Hz | `((18432000 * 73) / 14) / 2` |
| **sck** | Sample Clock | 24027428.57142857 Hz | `mck / 2` |
| **ick** | Index Clock | 3003428.571428571 Hz | `mck / 16` |

**Important**: Starting with KryoFlux Firmware 2.0+, the device transmits Hardware information that includes the values of these two clocks (see [KryoFlux Hardware Information](#kryoflux-hardware-information)). It is **recommended to use these values** as KryoFlux hardware may change these frequencies at some point in the future.

### Sample Counter

- **Width**: 16 bits
- **Purpose**: Measures the elapsed time between two flux reversals, or between a Flux reversal and an Index Signal
- **Overflow Handling**: Possible overflows are recorded (see Ovl16 block)
- **Reset Behavior**: Counter is reset after each Flux reversal recording

### Index Counter

- **Type**: "Free running" counter (not reset)
- **Purpose**: Records the value each time an Index Signal is detected
- **Reset Behavior**: Never reset

---

## Data Format

### Byte Alignment

Data in a Stream File is **byte-aligned** for processing efficiency. This means:

- No information is encoded at the bit level
- There is no need to break a byte down into bits for interpretation

### Byte Ordering

Data stored in 16 or 32-bit words uses **little-endian byte ordering**:

- The least significant byte first
- The most significant byte last

**Note**: This does **not** apply to Flux Blocks that use a specific encoding (see [Flux Blocks](#flux-blocks)).

---

## Stream File Structure

The data in a Stream File is organized in **Blocks** that have a variable length ranging from one to many bytes.

### Block Types

A stream file contains two types of Blocks:

1. **ISB (In Stream Buffer) blocks** - used to communicate the timing value of the sampled flux reversals
2. **OOB (Out Of stream Buffer) blocks** - used to help in the interpretation/verification of the Stream File as well as to transmit other critical information like Index Signals timing, or KryoFlux hardware information

### Critical Information

The most important information to retrieve from a Stream File:

- **Timing of Flux Reversals**: All data flux reversals detected by the KryoFlux device are stored in ISB Blocks
- **Timing of Index Signals**: All index signals detected by the KryoFlux device are transmitted in special OOB blocks (Index Blocks). This allows:
  - Computing the precise Index Time (time between two index signals)
  - Finding the Index Position in reference to the current data flux reversals

---

## Block Header

The first byte of a stream file Block is called the **Block Header**. It specifies how to interpret the Block.

### Block Header Values

The interpretation of the information contained in a Block of data depends on the Block Header. This header can take the following values (sorted in ascending order):

| Header | Name | Length | Description |
|--------|------|--------|-------------|
| `0x00-0x07` | Flux2 | 2 | Flux block: flux reversal count coded on two bytes |
| `0x08` | Nop1 | 1 | NOP block: Continue decoding at current position + 1 |
| `0x09` | Nop2 | 2 | NOP block: Continue decoding at current position + 2 |
| `0x0A` | Nop3 | 3 | NOP block: Continue decoding at current position + 3 |
| `0x0B` | Ovl16 | 1 | Flux block: next flux reversal count to be increased by 0x10000 |
| `0x0C` | Flux3 | 3 | Flux block: flux reversal count coded on three bytes |
| `0x0D` | OOB | variable | First byte of an Out Of stream Buffer block |
| `0x0E-0xFF` | Flux1 | 1 | Flux block: flux reversal count coded on one byte |

---

## ISB (In Stream Buffer) Blocks

An ISB Block is either a **Flux Blocks** or a **NOP Block** (i.e., not an OOB Block).

---

## Flux Blocks

A Flux Block is used to store the value of the Sample Counter. This corresponds to the number of Sample Clock Cycles (sck) between two flux reversals.

### Absolute Flux Timing

The flux reversal absolute timing values can be computed by dividing the sample counter value by the Sample Clock (sck):

```
AbsoluteFluxTiming = FluxValue / sck
```

### Flux1 Block

This block allows storing very efficiently the timing of a sampled flux reversal as it is coded on **only one byte**. The Block Header has a value in the range `0x0E-0xFF`.

**Structure:**
```
0x0E-0xFF
```

**Calculation:**
```
FluxValue = Header_value
```

**Note**: In practice, most flux reversal values fall into this range (0x0E-0xFF), contributing to very efficient coding of the stream file.

### Flux2 Block

This block allows storing the timing of a sampled flux reversal coded on **two bytes**. The Block Header has a value in the range `0x00-0x07`.

**Structure:**
```
0x00-0x07  Value1
```

**Calculation:**
```
FluxValue = (Header_value << 8) + Value1
```

### Flux3 Block

This block allows storing the timing of a sampled flux reversal coded on **three bytes**. The Block Header has a value equal to `0x0C`.

**Structure:**
```
0x0C  Value1  Value2
```

**Calculation:**
```
FluxValue = (Value1 << 8) + Value2
```

### Ovl16 Block

This block indicates that the next Flux Block has a value superior to the max value of a 16-bit number (0xFFFF). The Block Header has a value equal to `0x0B`.

**Structure:**
```
0x0B
```

**Calculation:**
```
FluxValue = 0x10000 + NextFluxValue
```

**Important Notes:**

- This block is inserted whenever the Sample Counter overflows
- There is **no limit** on the number of Ovl16 blocks present in a stream
- The maximum value for a flux reversal is virtually unlimited (decoder in KryoFlux host software uses a 32-bit value)
- Flux reversal values that do not fit into 16-bits are quite unusual but have been found in games that attempt to fool the AGC (Automatic Gain Control) of the drive electronics
- Multiple Ovl16 blocks can be chained to represent very large values

---

## NOP Blocks

A NOP (No-operation) Block is used to skip one or several byte(s) in the stream buffer. This makes it possible for the firmware to create data in its ring buffer without the need to break up a single code sequence when the filling of the ring buffer wraps.

### NOP1 Block

NOP1 block is used to skip **one byte** in the buffer. The Block Header is equal to `0x08`.

**Structure:**
```
0x08
```

**Action**: Just skip this byte during decoding.

### NOP2 Block

NOP2 block is used to skip **two bytes** in the buffer. The Block Header is equal to `0x09`.

**Structure:**
```
0x09  0xXX
```

**Action**: Just skip these two bytes during decoding.

### NOP3 Block

NOP3 block is used to skip **three bytes** in the buffer. The Block Header is equal to `0x0A`.

**Structure:**
```
0x0A  0xXX  0xYY
```

**Action**: Just skip these three bytes during decoding.

---

## OOB (Out Of stream Buffer) Blocks

An OOB Block is either used to:
- Help in the interpretation/verification of the stream file
- Transmit specific information (index signal, KryoFlux HW info)

**Important**: OOB blocks are sent **completely asynchronously** of the ISB blocks (see [KryoFlux Device Behavior](#kryoflux-device-behavior)).

### OOB Block Structure

An OOB Block is composed of:

1. **OOB Header Block** (always four bytes)
2. **OOB Data Block** (optional, variable length)

**OOB Block Header Structure:**
```
0x0D  Type  Size
```

**Fields:**
- **First field** (1 byte): Block Header, always equal to `0x0D`
- **Second field** (1 byte): Type of the OOB (see table below)
- **Third field** (2 bytes): Size of the optional OOB Data Block (little-endian)

**OOB Data Block**: Contains information specific to each Type of OOB Block

### OOB Block Types

| Type | Name | Meaning |
|------|------|---------|
| `0x00` | Invalid | Invalid OOB |
| `0x01` | StreamInfo | Stream Information (multiple per track) |
| `0x02` | Index | Index signal data |
| `0x03` | StreamEnd | No more flux to transfer (one per track) |
| `0x04` | KFInfo | HW Information from KryoFlux device |
| `0x0D` | EOF | End of file (no more data to process) |

### Invalid Block

It is not clear when this OOB Block is used, but it definitively indicates a problem.

**Structure:**
```
0x0D  0x00  0x0000
```

**Fields:**
- Type = `0x00`
- Size = `0x0000`

### StreamInfo Block

A StreamInfo block provides information on the progress of the data transfer. It is sent whenever the communication and the KryoFlux CPU budget allows it. The ordering of the StreamInfo blocks is guaranteed. It is possible to have several StreamInfo blocks at once.

**Purpose**:
- Check that no bytes have been lost during transmission
- Compute the transfer speed of the USB link between the host and the KryoFlux device

**Structure:**
```
0x0D  0x01  0x0008  Stream Position  Transfer Time
```

**Fields:**
- Type = `0x01`
- Size = `0x0008` (8 bytes - size of the following data block)
- **Stream Position** (4 bytes, little-endian): Indicates the position (in number of bytes) of the OOB Block Header in the stream buffer
- **Transfer Time** (4 bytes, little-endian): Gives the elapsed time (in milliseconds) since the last StreamInfo block

**Usage**: Can calculate transfer speed between the host and the board as well as the transfer's jitter.

### Index Block

This block is used to provide timing information about a detected index.

**Structure:**
```
0x0D  0x02  0x000C  Stream Position  Sample Counter  Index Counter
```

**Fields:**
- Type = `0x02`
- Size = `0x000C` (12 decimal - size of the following data block)
- **Stream Position** (4 bytes, little-endian): Indicates the position (in number of bytes) in the stream buffer of the next flux reversal just after the index was detected
- **Sample Counter** (4 bytes, little-endian): Gives the value of the Sample Counter when the index was detected. This is used to get accurate timing of the index in respect with the previous flux reversal. The timing is given in number of Sample Clock (sck). Note that it is possible that one or several sample counter overflows happen before the index is detected
- **Index Counter** (4 bytes, little-endian): Stores the value of the Index Counter when the index is detected. The value is given in number of Index Clock (ick). To get absolute timing values from the index counter values, divide these numbers by the index clock (ick)

For more information on index timing interpretation, see [Index Timing Considerations](#index-timing-considerations) and [Analysis of Index Information](#analysis-of-index-information).

### StreamEnd Block

A StreamEnd block indicates that all the Flux blocks have been transmitted. It also provides a KryoFlux status code that indicates if the streaming was done correctly by the hardware.

**Structure:**
```
0x0D  0x03  0x0008  Stream Position  Result Code
```

**Fields:**
- Type = `0x03`
- Size = `0x0008` (size of the data block)
- **Stream Position** (4 bytes, little-endian): Indicates the position (in number of bytes) of the OOB Block Header in the stream file
- **Hardware Status Code** (4 bytes, little-endian): Returns a value as defined below

#### Hardware Status Code Values

| Value | Name | Meaning |
|-------|------|---------|
| `0x00` | Ok | Transfer success (does not imply data is good, just that streaming was successful) |
| `0x01` | Buffer | Buffering problem - data transfer delivery to host could not keep up with disk read |
| `0x02` | No Index | No index signal detected |

### KFInfo Block

A KFInfo block is used to transmit information from the KryoFlux device to the host.

**Structure:**
```
0x0D  0x04  Size  Info Data (ASCII)
```

**Fields:**
- Type = `0x04`
- **Size** (2 bytes, little-endian): Number of bytes of the KFInfo data block (including the terminating null)
- **Info Data**: A null-terminated ASCII String of information

More details about Hardware Information transmitted can be found in the section [KryoFlux Hardware Information](#kryoflux-hardware-information).

### EOF Block

An EOF block is used to indicate the end of the stream file. No processing needs to be done beyond this block.

**Structure:**
```
0x0D  0x0D  0x0D0D
```

**Fields:**
- Type = `0x0D`
- Size = `0x0D0D` (not meaningful)

---

## Flux Data Encoding Optimization

The encoding scheme is optimized for efficiency:

### Range Allocations

- **Flux2 block** could be used to encode data in the range `0x0000-0x07FF`, but in practice it is more efficient to use a **Flux1 block** (only one byte) for encoding data in the range `0x000E-0x00FF`
- Therefore, **Flux2 is only used** to encode data in:
  - Range `0x0000-0x000D`, OR
  - Range `0x0100-0x07FF`
- For similar reasons (best efficiency), a **Flux3 block is only used** for encoding data in the range `0x0800-0xFFFF`
- If the flux reversal value to transmit is bigger than `0xFFFF`, then one or several **Ovl16 block(s)** is (are) used to add `0x10000` to the next flux reversal value

### Encoding Summary

| Value Range | Block Type | Bytes Used |
|-------------|------------|------------|
| `0x000E-0x00FF` | Flux1 | 1 byte |
| `0x0000-0x000D` | Flux2 | 2 bytes |
| `0x0100-0x07FF` | Flux2 | 2 bytes |
| `0x0800-0xFFFF` | Flux3 | 3 bytes |
| `> 0xFFFF` | Ovl16 + Flux block | 1+ bytes |

---

## Index Timing Considerations

Flux Reversal timing values recorded in a Stream File only make sense when the Index Signals positions are known. Once all of the data in the stream file has been processed, several computations are required on the index data.

### Index Position

The **Index Position** is the exact position where an Index Signal occurred in reference to the Flux Reversals. It can be determined during decoding by:

- Storing the position of all the flux reversals
- Storing the position where each index signal occurs

### Index Time

The **Index Time** is the time taken for one complete revolution of the disk. It can be calculated in two ways:

1. **Using Index Clock**: Equal to the number of index clock cycles since the last index occurred
   ```
   IndexTime = (IndexClock_n+1 - IndexClock_n) / ick
   ```

2. **Using Flux Reversals**: Sum all the flux reversal values recorded since the previous index, add the Sample Counter value at which the index was detected (see Sample Counter field in Index Block), and subtract the Sample Counter value of the previous index
   ```
   IndexTime = (Sum of all flux values between indexes + SampleCounter_current - SampleCounter_previous) / sck
   ```

**Note**: Until the first index, an Index Time cannot be generated as it will always be a partial revolution.

### RPM Calculation

The Index Time allows computing the exact floppy disk RPM value for one revolution. For example, for a drive that runs at 300 RPM, the time between two indexes should be 200ms.

**Formula:**
```
RPM = 60 / IndexTime_in_seconds
```

**Example:**
```
For 300 RPM: IndexTime = 60/300 = 0.2 seconds = 200ms
```

**Important**: From experience, the actual value differs, and it is therefore important to monitor the RPM for each revolution sampled.

The Index Position is also important, as it is the only marker on a disk that can be used to:
- Perfectly align data when writing
- Decide on the exact position of data when reading

---

## RPM Interpolation

To increase reliability, the decoding software can perform **RPM interpolation** when converting timing to absolute values.

### Problem

- If the RPM of one index is significantly different from the following index, the disk drive doing the reading may be unreliable
- Even if RPM is very stable, it may have been set incorrectly (e.g., 301 RPM instead of 300 RPM)
- This would affect all flux reversals across the track
- Since there are hundreds of thousands of samples, the differences will add up eventually

### Solution

Moderate these variations by converting each flux value using an interpolated value. Various interpolation algorithms are possible.

**Example Formula:**
```
CorrectedValue = OriginalValue * (Expected_RPM / Actual_RPM)
```

**Example:**
```
If Expected_RPM = 300 and Actual_RPM = 301:
CorrectedValue = OriginalValue * (300 / 301) = OriginalValue * 0.99668
```

---

## Decoding Stream Files

It is **recommended** to decode a KryoFlux stream file in **two passes**:

### First Pass

Used to parse the Stream File in order to retrieve and store all the important information:
- Flux timing and positioning
- Index timing and positioning

### Second Pass

Used to analyze the stored data in order to compute:
- Exact positioning of the Index Signals relative to flux reversals
- Index times
- Other derived metrics

### Clock Values

It is also recommended to check if KryoFlux hardware information about SCK and ICK has been passed. If this is the case, **these values should be used** rather than default clock values (see [KryoFlux Hardware Information](#kryoflux-hardware-information)).

---

## KryoFlux Device Behavior

To correctly process the data stored in a Stream File, it is useful to have a basic understanding of how the KryoFlux Device operates.

When imaging signals from a floppy drive reading a floppy disk, there are **two main processes** running independently in the KryoFlux device:

### 1. Sampling Process

The **sampling process** is responsible for capturing the data from the floppy drive and storing this information in a buffer called the **stream buffer**.

**Stream Buffer Contents:**
- Flux blocks (Flux1, Flux2, Flux3)
- Ovl16 blocks (overflow indicators)
- NOP blocks (considered as data without value)

**Behavior:**
- Each flux reversal value is stored as a Flux1, Flux2, or Flux3 block
- This value corresponds to the value of the Sample Counter at the time of the flux reversal
- Once recorded, the counter is reset
- Whenever the Sample Counter overflows, an Ovl16 block is stored
- The firmware can also add NOP blocks when necessary
- When an index signal is detected:
  - The information is **not** placed in the stream buffer
  - The position of the next flux reversal in the stream buffer is recorded
  - The value of the Sample Counter (time from previous flux reversal) is recorded
  - The Index Counter value is recorded

**Example Stream Buffer:**
```
Index Data:  F1  F1  F3  F2  F1  OV  F2  NOP3  F1  F1
```

Where:
- `F1` = Flux1 block
- `F2` = Flux2 block
- `F3` = Flux3 block
- `OV` = Ovl16 block
- `NOP3` = NOP3 block

### 2. Transfer Process

The **transfer process** is responsible for transferring the data from the KryoFlux device to the host over the USB link.

**Priority:**
1. **First priority**: Transmit the data stored in the stream buffer
2. **Second priority**: Transmit "extra information" (OOB Blocks) whenever communication and CPU budget allow

**OOB Block Insertion:**
- OOB blocks are **not part of the Stream Buffer**
- They are "inserted on the fly" by the transfer process
- Inserted between ISB blocks at **unpredictable times**
- Information in OOB Blocks is **completely asynchronous** from ISB Block information
- **Important**: It is possible to transmit information about an Index that refers to a flux reversal not yet transmitted!

**OOB Blocks Include:**
- Index information
- StreamInfo (transfer progress)
- StreamEnd (completion status)
- KFInfo (hardware information)

### Stopping Sampling

The sampling can be stopped:
- **Automatically**: After a certain amount of index signals
- **Programmatically**: Via a command (DTC may use both)

**Current Behavior:**
- Streaming is requested to stop after a specified number of indices
- If DTC detects certain errors, it may send a stop command at any time
- Stop command stops streaming as soon as possible (at some random location on the track)
- Even if sampling is to be stopped at an index signal, it may or may not stop immediately (depends on timing)

### Transfer Completion

The transfer process **always sends back all the data** that was sampled before signaling the transfer finished to the host:

- There may be one or more samples **after** the last index signal (if index stop mode used)
- Or there may be none

---

## KryoFlux Hardware Information

Starting with **version 2.0 of the firmware**, the KryoFlux device transmits information in one or several KFInfo blocks.

### Information Format

Most of the data transmitted are informative information about:
- Version of firmware
- Hardware information
- Host date/time
- Clock values

### Structure

- **Multiple Strings**: Several strings (usually two) can be passed from the KryoFlux device to the host
- **Null Terminated**: Each string is null-terminated
- **Size Field**: The size field in the KFInfo block gives the length of the string including the trailing null

### Data Format

The information inside the strings are passed as **"name" "value" pairs** separated by comma (`,`) character.

**Example:**
```
host_date=2012.01.22, host_time=17:44:47
```

### Important Information

Among the information transmitted, **two strings are particularly important**:

- **`sck`** (Sample Clock): Should be used instead of default value
- **`ick`** (Index Clock): Should be used instead of default value

### Example KFInfo Content

```
host_date=2011.03.21
host_time=17:20:17
name=KryoFlux DiskSystem
version=2.00
date=Mar 19 2011
time=14:35:18
hwid=1
hwrv=1
sck=24027428.5714285
ick=3003428.5714285625
```

---

## Parsing the Stream File

It is recommended to store the meaningful information in **arrays of structures** (Flux, Index, and Info) that can be queried by the target application.

### Recommended Data Structures

```
Flux Array:
  - fluxValue: Flux reversal timing value
  - streamPos: Position in stream buffer

Index Array:
  - streamPos: Position of next flux after index
  - sampleCounter: Sample Counter value when index detected
  - indexCounter: Index Counter value when index detected

Info Array:
  - String: KFInfo block content
```

**Note**: Memory management of the different arrays is not described here and should be handled by the application.

### Parsing Algorithm

Parsing is driven by the Block Header that defines the nature and length of the Blocks. All blocks are decoded in a loop that scans the complete Stream File until an **EOF block** is found.

Each Block is processed in **three steps**:

#### Step 1: Compute Block Length

Compute the length of the Block based on the header type. This information is used to move the pointer to the next block:

| Block Type | Length |
|------------|--------|
| Flux1, Nop1, Ovl16 | 1 byte |
| Flux2, Nop2 | 2 bytes |
| Flux3, Nop3 | 3 bytes |
| OOB Block | 4 bytes (OOB Header) + Size field value (OOB Data) |
| EOF Block | Size field not meaningful |

#### Step 2: Compute Flux Value

Compute the actual value of the flux reversal when the block is of type:
- Flux1: `FluxValue = Header_value`
- Flux2: `FluxValue = (Header_value << 8) + Value1`
- Flux3: `FluxValue = (Value1 << 8) + Value2`
- Ovl16: Add `0x10000` to the **next** Flux Block value

#### Step 3: Process the Block

Process based on block type:

- **Flux1, Flux2, Flux3**: Create a new entry in Flux array and store:
  - Flux Value
  - Stream Position
- **StreamInfo**:
  - Use Stream Position information to check that no bytes were lost during transmission
  - Use Transfer Time for statistical analysis of transfer speed
- **Index**: Create a new entry in Index array and store:
  - Stream Position
  - Sample Counter value
  - Index Clock value
- **KFInfo**: Copy the information into a String
- **StreamEnd**:
  - Use Stream Position information to check that no bytes were lost during transmission
  - Check Result Code to verify that no errors were found during processing
- **EOF**: Stop parsing the file

### After Parsing

When parsing of the stream file is finished, you have all the data information in three arrays (Flux, Index, and KFInfo), but you still need to **analyze the Index information** as explained in the next section.

---

## Analysis of Index Information

It is extremely important to:
1. Position the different Index Signals in respect with the flux reversals (and vice versa)
2. Measure the exact elapsed time between two Index Signals

For that matter, you need to perform some analysis on the stored data.

### Data Representation

For each flux reversal, store:
- **fluxValue**: The Sample Counter value
- **streamPos**: Position in stream buffer

For each Index Signal, store:
- **streamPos**: Position of next flux reversal in stream buffer when index was detected
- **sampleCounter**: Sample Counter value when index detected
- **indexCounter**: Index Counter value when index detected

### Index Time Calculation

Looking at timing information close to two adjacent Index Signals:

```
Index Signal n                          Index Signal n+1
     |                                        |
     |<---------- Index Time --------------->|
     |                                        |
     Timer_n                                  Timer_n+1
     |                                        |
Flux Reversals:  F1  F2  F3  ...  Fn
```

#### Method 1: Using Flux Reversals

Sum all flux reversals values between the two Index Signals, then:
- Subtract the Timer value of the first index signal
- Add the Timer value of the second index signal

**Formula:**
```
IndexTime = (Sum of flux values between indexes - SampleCounter_first + SampleCounter_second) / sck
```

All timing values are given in number of sample clocks.

#### Method 2: Using Index Clock

Take the Index Clock value of the second index and subtract the Index Clock value of the first index.

**Formula:**
```
IndexTime = (IndexCounter_second - IndexCounter_first) / ick
```

This gives the number of index clock cycles between the two index signals.

### Edge Cases

There are several **marginal conditions** for Index signals that you should consider:

#### 1. Sample Counter Overflows before Index

Some complexity arises if what was written last in the stream buffer is an overflow (Ovl16 block).

**Handling:**
- The stream and index decoder should take care of these cases
- The stream decoder has to find the "real" stream position while decoding the data
- The index decoder has to find the correct index referenced
- This is somewhat tricky because at this point flux reversals are already decoded, so they only ever are represented by one value
- The index decoder checks the range of stream positions elapsed between two cells

#### 2. Index Pointing after Last Flux

The KryoFlux firmware always points to the next position to be written by the sampler.

**Handling:**
- The stream decoder should add an extra empty flux at the end of the stream
- This flux is not made part of the decoded stream at this point since we don't know if it happened or not, without decoding the index data
- If the index analyzer detects that the index was pointing to a non-existent flux, it has to "activate" the empty flux added above

#### 3. Index Detected before Any Flux

There is another edge case when an index signal is detected but there is **no previous flux reversal**.

**Handling:**
- This should be handled as a special case in the decoder
- The decoder should account for this scenario when calculating Index Time

---

## Terminology

- **Flux Reversal**: A flux reversal or flux transition under the floppy drive head. This is referred to as a **cell** in the original SPS documentation.

- **ISB Blocks**: Any Blocks that are not OOB blocks (i.e., with a Block Header different from `0x0D`). In Stream Buffer blocks contain flux reversal information placed in the stream buffer by the KryoFlux sampling process. This is referred to as **stream data** in the original SPS documentation.

- **OOB Blocks**: Out Of stream Buffer blocks are used to transmit Index/hardware information or to help in decoding stream file. OOB Blocks have a Block Header equal to `0x0D`. They contain extra information (not in the stream buffer) transferred to the host by the KryoFlux transfer process. This is referred to as **Out Of Band** in the original SPS documentation.

- **Stream Position**: Position in the original KryoFlux stream buffer (i.e., the buffer prior to the insertion of the OOB blocks).

---

## References

- **SPS KryoFlux Project Presentation**
- **SPS Stream Protocol**
- **Original Documentation**: KryoFlux Stream Protocol Revision 1.1 by Jean Louis-Guérin (01/12/2013)
- **Based on**: DTC Version 2.2
