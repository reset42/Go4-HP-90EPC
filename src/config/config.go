package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

const AppName = "hp90epc"

type Config struct {
	DevicePort string `json:"device_port"`
	Baud       int    `json:"baud"`

	LogDir     string `json:"log_dir"`
	LogIntervalMs int  `json:"log_interval_ms"`

	HTTPAddr   string `json:"http_addr"`
}

func Default() Config {
	c := Config{
		DevicePort: defaultPortForOS(),
		Baud:       2400,
		LogDir:     "logs",
		LogIntervalMs: 1000,
		HTTPAddr:   ":8080",
	}
	return c
}

func defaultPortForOS() string {
	switch runtime.GOOS {
	case "windows":
		return "COM3"
	case "darwin":
		// macOS: meistens /dev/tty.usbserial-* oder /dev/tty.usbmodem-*
		// Default ist nur „Platzhalter“, UI/API soll das setzen können.
		return "/dev/tty.usbserial-0001"
	default:
		return "/dev/ttyUSB0"
	}
}

func ResolveAppDir(appdirFlag string, portable bool) (string, error) {
	if appdirFlag != "" {
		return filepath.Clean(appdirFlag), nil
	}
	if portable {
		exe, err := os.Executable()
		if err != nil {
			return "", err
		}
		return filepath.Dir(exe), nil
	}
	// OS Standard
	switch runtime.GOOS {
	case "windows":
		// %AppData%\hp90epc
		base := os.Getenv("APPDATA")
		if base == "" {
			return "", errors.New("APPDATA not set")
		}
		return filepath.Join(base, AppName), nil
	case "darwin":
		// ~/Library/Application Support/hp90epc
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", AppName), nil
	default:
		// Linux: ~/.config/hp90epc
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".config", AppName), nil
	}
}

func ConfigPath(appDir string) string {
	return filepath.Join(appDir, "config.json")
}

func Load(appDir string) (Config, error) {
	_ = os.MkdirAll(appDir, 0o755)

	path := ConfigPath(appDir)
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			c := Default()
			// gleich schreiben, damit’s „greifbar“ ist
			_ = Save(appDir, c)
			return c, nil
		}
		return Config{}, err
	}

	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		// kaputte Datei → Default liefern, aber Datei NICHT überschreiben
		return Default(), err
	}

	// kleine Defaults für fehlende Felder
	def := Default()
	if c.DevicePort == "" {
		c.DevicePort = def.DevicePort
	}
	if c.Baud == 0 {
		c.Baud = def.Baud
	}
	if c.LogDir == "" {
		c.LogDir = def.LogDir
	}
	if c.LogIntervalMs == 0 {
		c.LogIntervalMs = def.LogIntervalMs
	}
	if c.HTTPAddr == "" {
		c.HTTPAddr = def.HTTPAddr
	}

	return c, nil
}

func Save(appDir string, c Config) error {
	_ = os.MkdirAll(appDir, 0o755)

	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := ConfigPath(appDir) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, ConfigPath(appDir))
}

