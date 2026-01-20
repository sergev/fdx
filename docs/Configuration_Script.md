# Configuration Script

The floppy tool uses a TOML configuration file to define floppy drive parameters and available disk images. The configuration file is automatically created on first run from an embedded default template.

## File Location

The configuration file is stored as `~/.floppy` on Linux and macOS, or in `%APPDATA%\floppy\.floppy` on Windows.

If the file doesn't exist when the tool starts, it will be automatically created from the default configuration.

## Purpose

The configuration file serves two main purposes:

1. **Drive Configuration**: Defines the physical characteristics of your floppy drive (cylinders, heads, rotation speed, maximum bit rate)
2. **Image Registry**: Maps user-friendly image names to disk image files that can be used with the `format` command

## Syntax

The configuration file uses the [TOML (Tom's Obvious, Minimal Language)](https://toml.io/en/) format. For detailed syntax information, refer to the [TOML Specification](https://toml.io/en/latest).

### Configuration Structure

The file consists of three main sections:

#### 1. Default Drive

```toml
default = "3.5-inch 1.44M"
```

Specifies which drive configuration to use by default. The value must match the `name` field of one of the drive entries defined below.

#### 2. Drive Definitions

Each `[[drive]]` section defines a floppy drive type:

```toml
[[drive]]
    name = "3.5-inch 1.44M"
    cyls = 80
    heads = 2
    rpm = 300
    maxkbps = 500
    images = [
        "MS-DOS 1.44M",
        "MS-DOS 1.6M",
        "Linux 1.44M",
        "BSD 1.44M",
    ]
```

**Drive Fields:**

- `name` (string, required): A unique identifier for this drive type (e.g., "3.5-inch 1.44M", "5.25-inch 360K")
- `cyls` (integer, required): Number of cylinders (tracks per side). Must be a positive integer.
- `heads` (integer, required): Number of read/write heads (sides). Must be a positive integer (typically 1 or 2).
- `rpm` (integer, required): Rotation speed in revolutions per minute. Common values are 300 or 360 RPM.
- `maxkbps` (integer, required): Maximum bit rate in kilobits per second that the drive supports. Common values:
  - 250 kbps: Standard double density (DD) drives
  - 500 kbps: Standard high density (HD) drives
  - 1000 kbps: Extended density (ED) drives
- `images` (array of strings, required): List of image names (defined in the `[[image]]` sections) that are compatible with this drive. This list determines which formats are available when using the `format` command with this drive.

#### 3. Image Definitions

Each `[[image]]` section maps a user-friendly name to a disk image file:

```toml
[[image]]
    name = "MS-DOS 1.44M"
    file = "fat1.44.img"
```

**Image Fields:**

- `name` (string, required): The display name used in the `format` command menu. Must be unique.
- `file` (string, required): The filename of the embedded image file (without the `.gz` extension). The actual file must exist in the `images/` directory as a gzip-compressed file (e.g., `fat1.44.img.gz`).

**Note:** Image names referenced in drive `images` arrays must have corresponding `[[image]]` entries.

## Example Configuration

```toml
default = "3.5-inch 1.44M"

[[drive]]
    name = "3.5-inch 1.44M"
    cyls = 80
    heads = 2
    rpm = 300
    maxkbps = 500
    images = [
        "MS-DOS 1.44M",
        "MS-DOS 1.6M",
        "Linux 1.44M",
        "BSD 1.44M",
    ]

[[drive]]
    name = "5.25-inch 360K"
    cyls = 40
    heads = 2
    rpm = 300
    maxkbps = 250
    images = [
        "MS-DOS 360K",
        "MS-DOS 400K",
    ]

[[image]]
    name = "MS-DOS 1.44M"
    file = "fat1.44.img"

[[image]]
    name = "MS-DOS 360K"
    file = "fat360.img"

[[image]]
    name = "Linux 1.44M"
    file = "linux1.44.img"
```

## Validation

The configuration file is validated when the tool starts. The following checks are performed:

1. The `default` key must be present and non-empty
2. The default drive name must exist in the drive array
3. All drive fields must have positive integer values
4. Each drive must have at least one image in its `images` array
5. All image names referenced in drive `images` arrays must have corresponding `[[image]]` entries

If any validation fails, the tool will report an error and exit.

## References

- [TOML Specification](https://toml.io/en/latest) - Official TOML format documentation
- [TOML v1.0.0](https://toml.io/en/v1.0.0) - Current TOML standard
