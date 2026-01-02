## Overview

* Connector: **34-pin IDC ribbon**, 2×17
* All **odd-numbered pins are GND**
* Signals are **TTL, active-low**, usually **open-collector** on the drive
* Direction is given as **Host → Drive** or **Drive → Host**

---

## Pinout Table (PC-Compatible Shugart)

| Pin | Name                  | Direction    | Meaning                                    |
| --: | --------------------- | ------------ | ------------------------------------------ |
|   2 | **/DSKCHG** (or /RDY) | Drive → Host | Disk Change (PC); Ready (original Shugart) |
|   4 | **/N/C**              | —            | Not connected (reserved)                   |
|   6 | **/DS3**              | Host → Drive | Drive Select 3 (rarely used on PCs)        |
|   8 | **/INDEX**            | Drive → Host | Index pulse once per revolution            |
|  10 | **/DS0**              | Host → Drive | Drive Select 0                             |
|  12 | **/DS1**              | Host → Drive | Drive Select 1                             |
|  14 | **/DS2**              | Host → Drive | Drive Select 2                             |
|  16 | **/MOTOR ON**         | Host → Drive | Turn spindle motor on                      |
|  18 | **/DIR**              | Host → Drive | Step direction (1 = inward, 0 = outward)   |
|  20 | **/STEP**             | Host → Drive | Step pulse (one track per pulse)           |
|  22 | **/WRDATA**           | Host → Drive | Write data (MFM/FM encoded)                |
|  24 | **/WRGATE**           | Host → Drive | Enables writing                            |
|  26 | **/TRK0**             | Drive → Host | Track 0 indicator                          |
|  28 | **/WRPROT**           | Drive → Host | Write protect (media tab)                  |
|  30 | **/RDATA**            | Drive → Host | Read data (raw flux transitions)           |
|  32 | **/SIDE1**            | Host → Drive | Head select (0 = side 0, 1 = side 1)       |
|  34 | **/DISKCHG**          | Drive → Host | Disk Change (PC standard)                  |

On Greaseweazle, only pins 2, 4, 6, 8, 26, 28 and 34 are readable.

---

## Important Signal Explanations

### 1. Drive Selects (/DS0–/DS3)

* Only **one drive responds at a time**
* PCs typically use **DS0 and DS1 only**
* IBM PC twist swaps DS0/DS1 → logical A:/B:

### 2. Disk Change vs Ready (Pin 2 / Pin 34)

This is a classic source of confusion:

| System           | Pin 2        | Pin 34       |
| ---------------- | ------------ | ------------ |
| Original Shugart | /READY       | /DISK CHANGE |
| IBM PC / AT      | /DISK CHANGE | /DISK CHANGE |
| Many 3.5″ drives | /DISK CHANGE | /DISK CHANGE |

Modern PC drives often **do not output READY at all**.

---

### 3. Motor Control (/MOTOR ON)

* Single motor control line
* Drive spins when **selected AND MOTOR ON is asserted**
* Older Shugart drives had **separate motor lines per drive**

---

### 4. Step and Direction

* `/DIR` sets direction
* `/STEP` pulse causes head movement
* Timing critical (typically ≥1 µs pulse width)

---

### 5. Read & Write Signals

* `/RDATA`: raw bitstream from the drive
* `/WRDATA`: raw encoded bitstream to drive
* `/WRGATE`: enables writing (must be active)

Flux-level devices (Greaseweazle, KryoFlux) sample `/RDATA` timing.

---

### 6. Index Pulse (/INDEX)

* One pulse per disk revolution
* Used for synchronization, formatting, RPM detection

---

### 7. Track 0 (/TRK0)

* Asserted when head is at outermost track
* Used for recalibration

---

### 8. Write Protect (/WRPROT)

* Indicates write-protected media
* Active if tab is open (3.5″) or notch covered (5.25″)

---

### 9. Side Select (/SIDE1)

* Selects upper/lower head
* Ignored on single-sided drives

---

## Electrical Notes

* Signals are **active-low**
* Drive outputs are usually **open collector**
* Pull-ups exist on the controller side
* Cable length matters (esp. with flux samplers)

---

## Variants & Gotchas

* **READY signal missing** on many PC drives
* **Twisted cable** swaps DS lines (pins 10–16 region)
* Some drives repurpose pins via jumpers
* Non-PC Shugart systems (CP/M, S-100) differ slightly
