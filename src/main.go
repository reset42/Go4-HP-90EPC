package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"hp90epc/config"
	"hp90epc/logging"
	"hp90epc/model"
	"hp90epc/reader"
	"hp90epc/server"
)

type app struct {
	latest *model.LatestBuffer
	mgr    *reader.Manager
	logger *logging.Logger

	cfg    config.Config
	appDir string
	cfgMu  sync.Mutex
}

func (a *app) GetLatest() *model.Measurement  { return a.latest.Get() }
func (a *app) GetReaderStatus() reader.Status { return a.mgr.GetStatus() }
func (a *app) SetDevice(port string, baud int) error {
	if err := a.mgr.SetPort(port, baud); err != nil {
		return err
	}
	a.cfgMu.Lock()
	a.cfg.DevicePort = port
	a.cfg.Baud = baud
	a.cfgMu.Unlock()
	a.saveConfig()
	return nil
}
func (a *app) GetLogStatus() logging.LogStatus { return a.logger.Status() }
func (a *app) LogStart() (logging.LogStatus, error) {
	err := a.logger.Start()
	return a.logger.Status(), err
}

func (a *app) LogStop() (logging.LogStatus, error) {
	err := a.logger.Stop()
	return a.logger.Status(), err
}
func (a *app) LogSetInterval(ms int) error {
	a.logger.SetInterval(ms)
	a.cfgMu.Lock()
	a.cfg.LogIntervalMs = ms
	a.cfgMu.Unlock()
	a.saveConfig()
	return nil
}
func (a *app) LogListFiles() ([]string, error)              { return a.logger.ListFiles() }
func (a *app) LogReadFile(name string) ([]byte, error)      { return a.logger.ReadFile(name) }
func (a *app) LogTail(name string, n int) ([]string, error) { return a.logger.Tail(name, n) }

func (a *app) saveConfig() {
	if a.appDir == "" {
		return
	}
	a.cfgMu.Lock()
	cfg := a.cfg
	a.cfgMu.Unlock()
	if err := config.Save(a.appDir, cfg); err != nil {
		log.Printf("warn: save config: %v", err)
	}
}

func main() {
	port := flag.String("port", defaultPort(), "serial port for HP-90EPC (e.g. /dev/ttyUSB0 or COM3)")
	baud := flag.Int("baud", 2400, "serial baud rate")
	httpAddr := flag.String("http", ":8080", "HTTP listen address")
	logDir := flag.String("logdir", "logs", "directory for CSV log files")
	intervalMs := flag.Int("log-interval-ms", 1000, "logging interval in milliseconds")
	appdirFlag := flag.String("appdir", "", "custom app dir for config/logs")
	portable := flag.Bool("portable", false, "store config/logs next to the binary")
	noBrowser := flag.Bool("no-browser", false, "do not auto-open browser")

	setFlags := map[string]bool{}
	flag.Parse()
	flag.CommandLine.Visit(func(f *flag.Flag) {
		setFlags[f.Name] = true
	})

	appDir, err := config.ResolveAppDir(*appdirFlag, *portable)
	if err != nil {
		log.Fatalf("resolve app dir: %v", err)
	}

	cfg, err := config.Load(appDir)
	if err != nil {
		log.Printf("warn: load config: %v (using defaults)", err)
		cfg = config.Default()
	}

	if setFlags["port"] {
		cfg.DevicePort = *port
	}
	if setFlags["baud"] {
		cfg.Baud = *baud
	}
	if setFlags["http"] {
		cfg.HTTPAddr = *httpAddr
	}
	if setFlags["logdir"] {
		cfg.LogDir = *logDir
	}
	if setFlags["log-interval-ms"] {
		cfg.LogIntervalMs = *intervalMs
	}

	// persist merged config
	if err := config.Save(appDir, cfg); err != nil {
		log.Printf("warn: save config: %v", err)
	}

	resolvedLogDir := cfg.LogDir
	if !filepath.IsAbs(resolvedLogDir) {
		wd, _ := os.Getwd()
		appDirLog := filepath.Join(appDir, resolvedLogDir)
		cwdLog := filepath.Join(wd, resolvedLogDir)
		switch {
		case pathExists(cwdLog):
			resolvedLogDir = cwdLog
		case pathExists(appDirLog):
			resolvedLogDir = appDirLog
		default:
			resolvedLogDir = appDirLog
		}
	}

	latest := &model.LatestBuffer{}
	logger := logging.NewLogger(resolvedLogDir, time.Duration(cfg.LogIntervalMs)*time.Millisecond)
	mgr := reader.NewManager(latest, logger, 3*time.Second)

	// Reader starten (nicht fatal, wenn Multi nicht da ist)
	_ = mgr.Start(cfg.DevicePort, cfg.Baud)

	app := &app{
		latest: latest,
		mgr:    mgr,
		logger: logger,
		cfg:    cfg,
		appDir: appDir,
	}

	go func() {
		if err := server.Start(cfg.HTTPAddr, app); err != nil {
			log.Fatalf("http server: %v", err)
		}
	}()

	if !*noBrowser {
		go func() {
			time.Sleep(600 * time.Millisecond)
			url := urlFromAddr(*httpAddr)
			_ = openBrowser(url)
		}()
	}

	log.Printf("HP-90EPC started. HTTP=%s Device=%s@%d AppDir=%s", cfg.HTTPAddr, cfg.DevicePort, cfg.Baud, appDir)

	// block forever
	select {}
}

func defaultPort() string {
	switch runtime.GOOS {
	case "windows":
		return "COM3"
	case "darwin":
		// typische Namen: /dev/tty.usbserial-XXXX oder /dev/cu.usbserial-XXXX
		return "/dev/tty.usbserial"
	default:
		return "/dev/ttyUSB0"
	}
}

func urlFromAddr(addr string) string {
	a := strings.TrimSpace(addr)
	if a == "" {
		return "http://localhost:8080/"
	}
	if strings.HasPrefix(a, ":") {
		return "http://localhost" + a + "/"
	}
	if strings.HasPrefix(a, "http://") || strings.HasPrefix(a, "https://") {
		if strings.HasSuffix(a, "/") {
			return a
		}
		return a + "/"
	}
	return "http://" + a + "/"
}

func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		return fmt.Errorf("unsupported OS %q", runtime.GOOS)
	}
	return cmd.Start()
}

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
