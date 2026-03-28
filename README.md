## Transcode Daemon

Transcode Daemon watches an input tree and mirrors it into an output tree:

- Audio files are transcoded to AAC 320kbps `.m4a`.
- Non-audio files are copied as-is.
- Video files are ignored.

The daemon preserves relative directory structure from input to output.

## Usage

You can run Transcode Daemon using the Docker container image published to `ghcr.io/mhwdvs/transcode-daemon:latest`.

Example `docker-compose.yml` service definition:

```yaml
transcode-daemon:
    container_name: transcode-daemon
    image: ghcr.io/mhwdvs/transcode-daemon:latest
    volumes:
      - local/path/to/your/input:/input
      - local/path/to/your/output:/output
    command: ["-input", "/input", "-output", "/output"]
```

## CLI flags

- `-input` (default `./input`): input root directory
- `-output` (default `./output`): output root directory
- `-dry-run`: log actions without writing files
- `-overwrite`: allow replacing existing destination files
- `-help`: print usage

### Overwrite policy

- Default (`-overwrite=false`): existing destination files are skipped.
  - Audio skip checks use the canonical output path (`.m4a`) derived from the input relative path.
  - FFmpeg is invoked with `-n` (never overwrite).
- With `-overwrite=true`: destination files are replaced.
  - FFmpeg is invoked with `-y`.

## Event model and processing semantics

The daemon performs an initial scan and then live watch processing.

### Initial scan

- Recursively processes existing files under input root.
- Uses canonical destination mapping based on relative path from input root.

### Watch mode

- Watches all existing subdirectories and dynamically adds newly created directories to the watcher.
- Handles `Create`, `Write`, and `Rename` events.
- Ignores directories for file processing (directories are only used for watcher expansion).
- Uses non-blocking event ingestion:
  1. fsnotify goroutine receives events,
  2. buffered queue + debounce stage coalesces bursts by path,
  3. worker goroutines process stabilized files.
- Applies per-path debounce window to collapse duplicate bursts.
- Applies a file stability gate before processing watched files:
  - waits until size + mtime remain stable for a minimum window,
  - aborts after a bounded timeout if stability is not reached.

## Build and run

```bash
cd transcode-daemon
go build -o transcode-daemon
./transcode-daemon -input /path/to/input -output /path/to/output
```

Dry-run example:

```bash
./transcode-daemon -input /path/to/input -output /path/to/output -dry-run
```

Overwrite example:

```bash
./transcode-daemon -input /path/to/input -output /path/to/output -overwrite
```

## Dependencies

- `ffmpeg` must be installed and available on `PATH`.
- `fsnotify` is used for recursive directory watch management.
