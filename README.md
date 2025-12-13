# Go4-HP-90EPC (v1)

Compact HTTP server and web UI for the HP-90EPC (serial - Multimeter), with live display and CSV logging. Forks for other meters are explicitly welcome.

![Preview](assets/ui/preview.png)

## Highlights
- Stream parser for the serial feed (no blocking `readExact`), near real-time vs. the device display.
- Stale-based `connected` flag and reader status API.
- Web UI with Port/Baud modal, Logging modal (interval set, list files, CSV download, tail inline), live value plus raw hex.
- Persistent config (DevicePort, Baud, LogDir, LogIntervalMs, HTTPAddr) per OS default path, overridable via flags/portable.

## Build
```bash
go build -o hp90epc
```

Cross-compile examples:
```bash
GOOS=windows GOARCH=amd64 go build -o hp90epc.exe
GOOS=darwin  GOARCH=amd64 go build -o hp90epc-macos-amd64
GOOS=darwin  GOARCH=arm64 go build -o hp90epc-macos-arm64
GOOS=linux   GOARCH=amd64 go build -o hp90epc-linux-amd64
```

## Run
```bash
./hp90epc
```

Key flags:
- `--port` / `--baud`         Serial device
- `--http`                    HTTP listen (default `:8080`)
- `--appdir`                  Force app dir for config/logs
- `--portable`                Keep config/logs next to the binary
- `--logdir`                  Logging directory (relative → resolved to `appdir` or CWD)
- `--log-interval-ms`         Logging interval in ms
- `--no-browser`              Skip auto-open

Config is written automatically (defaults):
- Linux: `~/.config/hp90epc/config.json`
- macOS: `~/Library/Application Support/hp90epc/config.json`
- Windows: `%AppData%\hp90epc\config.json`

## Logging
- Files: `hp90epc_YYYY-MM-DD_HH-MM-SS.csv` in the log dir.
- UI: start/stop, set interval, list files, CSV download, tail inline.
- API: `/api/log/status|start|stop|interval|files|file|tail`.

## API Quick Reference
- Live: `/api/live`
- Reader status: `/api/reader/status` (stale-based)
- Device hot-swap: `POST /api/device/port` `{port, baud}`
- Logging: see above.

## License
MIT License — Copyright (c) 2025 Andreas Pirsig @ Reset42  
Forks and adaptations for other devices are welcome.
