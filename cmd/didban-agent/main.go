package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/coder/websocket"
)

var version = "dev"

type config struct {
	serverURL string
	apiKey    string
	agentID   string
	name      string
	interval  time.Duration
	hostname  string
}

type cpuTime struct {
	idle  uint64
	total uint64
}

type cpuSnapshot struct {
	total cpuTime
	cores []cpuTime
}

type metrics struct {
	CPUPercent          float64   `json:"cpuPercent"`
	CPUCoresPercent     []float64 `json:"cpuCoresPercent"`
	MemoryUsedBytes     uint64    `json:"memoryUsedBytes"`
	MemoryTotalBytes    uint64    `json:"memoryTotalBytes"`
	ProcessUptimeSecond float64   `json:"processUptimeSeconds"`
	SystemUptimeSecond  float64   `json:"systemUptimeSeconds"`
	LoadAverage1        float64   `json:"loadAverage1"`
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("didban-agent %s starting as %s", version, cfg.agentID)
	for attempt := 0; ctx.Err() == nil; attempt++ {
		if err := run(ctx, cfg); err != nil && ctx.Err() == nil {
			delay := time.Second << min(attempt, 5)
			log.Printf("connection lost: %v; retrying in %s", err, delay)
			select {
			case <-time.After(delay):
			case <-ctx.Done():
			}
			continue
		}
		attempt = 0
	}
}

func loadConfig() (config, error) {
	hostname, _ := os.Hostname()
	serverURL, err := resolveSocketURL(env("DIDBAN_WS_URL", "wss://api.getdidban.ir/api/v1/live"))
	if err != nil {
		return config{}, err
	}
	apiKey := strings.TrimSpace(os.Getenv("DIDBAN_API_KEY"))
	if apiKey == "" {
		return config{}, errors.New("DIDBAN_API_KEY is required")
	}
	interval, err := strconv.Atoi(env("DIDBAN_AGENT_INTERVAL_MS", "5000"))
	if err != nil || interval < 1000 {
		return config{}, errors.New("DIDBAN_AGENT_INTERVAL_MS must be at least 1000")
	}
	return config{
		serverURL: serverURL,
		apiKey:    apiKey,
		agentID:   env("DIDBAN_AGENT_ID", hostname+"-"+runtime.GOOS+"-"+runtime.GOARCH),
		name:      env("DIDBAN_AGENT_NAME", hostname),
		interval:  time.Duration(interval) * time.Millisecond,
		hostname:  hostname,
	}, nil
}

func resolveSocketURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("invalid DIDBAN_WS_URL: %w", err)
	}
	switch parsed.Scheme {
	case "http":
		parsed.Scheme = "ws"
	case "https":
		parsed.Scheme = "wss"
	case "ws", "wss":
	default:
		return "", errors.New("DIDBAN_WS_URL must use ws, wss, http or https")
	}
	if parsed.Host == "" {
		return "", errors.New("DIDBAN_WS_URL must include a host")
	}
	if parsed.Path == "" || parsed.Path == "/" || strings.TrimSuffix(parsed.Path, "/") == "/api" {
		parsed.Path = "/api/v1/live"
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func run(ctx context.Context, cfg config) error {
	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	conn, _, err := websocket.Dial(dialCtx, cfg.serverURL, nil)
	cancel()
	if err != nil {
		return err
	}
	defer conn.CloseNow()

	auth := map[string]any{
		"type":   "auth",
		"role":   "agent",
		"apiKey": cfg.apiKey,
		"agent": map[string]string{
			"id": cfg.agentID, "name": cfg.name, "version": version,
			"hostname": cfg.hostname, "platform": platform(),
		},
	}
	if err := writeJSON(ctx, conn, auth); err != nil {
		return err
	}
	authCtx, authCancel := context.WithTimeout(ctx, 15*time.Second)
	_, message, err := conn.Read(authCtx)
	authCancel()
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}
	var response struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(message, &response) != nil || response.Type != "auth.ok" {
		return errors.New("server rejected authentication")
	}
	log.Printf("connected to %s", cfg.serverURL)

	readErrors := make(chan error, 1)
	go func() {
		for {
			_, payload, readErr := conn.Read(ctx)
			if readErr != nil {
				readErrors <- readErr
				return
			}
			var event struct {
				Type string `json:"type"`
				Code string `json:"code"`
			}
			if json.Unmarshal(payload, &event) == nil && event.Type == "error" {
				log.Printf("server error: %s", event.Code)
			}
		}
	}()

	startedAt := time.Now()
	previous, _ := readCPU("/proc/stat")
	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = conn.Close(websocket.StatusNormalClosure, "service stopping")
			return nil
		case err := <-readErrors:
			return err
		case <-ticker.C:
			current, cpuErr := readCPU("/proc/stat")
			if cpuErr != nil {
				return cpuErr
			}
			memoryUsed, memoryTotal, _ := readMemory("/proc/meminfo")
			systemUptime, _ := readFirstFloat("/proc/uptime")
			loadAverage, _ := readFirstFloat("/proc/loadavg")
			payload := map[string]any{
				"type": "telemetry", "timestamp": time.Now().UTC().Format(time.RFC3339Nano), "level": "info",
				"metrics": metrics{
					CPUPercent:          percentage(previous.total, current.total),
					CPUCoresPercent:     corePercentages(previous.cores, current.cores),
					MemoryUsedBytes:     memoryUsed,
					MemoryTotalBytes:    memoryTotal,
					ProcessUptimeSecond: time.Since(startedAt).Seconds(),
					SystemUptimeSecond:  systemUptime,
					LoadAverage1:        loadAverage,
				},
			}
			previous = current
			if err := writeJSON(ctx, conn, payload); err != nil {
				return err
			}
		}
	}
}

func writeJSON(ctx context.Context, conn *websocket.Conn, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return conn.Write(writeCtx, websocket.MessageText, payload)
}

func readCPU(path string) (cpuSnapshot, error) {
	file, err := os.Open(path)
	if err != nil {
		return cpuSnapshot{}, err
	}
	defer file.Close()
	result := cpuSnapshot{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 5 || !strings.HasPrefix(fields[0], "cpu") {
			if len(fields) > 0 && !strings.HasPrefix(fields[0], "cpu") {
				break
			}
			continue
		}
		item := cpuTime{}
		for index, value := range fields[1:] {
			number, _ := strconv.ParseUint(value, 10, 64)
			item.total += number
			if index == 3 || index == 4 {
				item.idle += number
			}
		}
		if fields[0] == "cpu" {
			result.total = item
		} else {
			result.cores = append(result.cores, item)
		}
	}
	return result, scanner.Err()
}

func percentage(previous, current cpuTime) float64 {
	if current.total < previous.total || current.idle < previous.idle {
		return 0
	}
	total := current.total - previous.total
	idle := current.idle - previous.idle
	if total == 0 || idle > total {
		return 0
	}
	return round1(float64(total-idle) / float64(total) * 100)
}

func corePercentages(previous, current []cpuTime) []float64 {
	result := make([]float64, len(current))
	for index := range current {
		if index < len(previous) {
			result[index] = percentage(previous[index], current[index])
		}
	}
	return result
}

func readMemory(path string) (uint64, uint64, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()
	values := map[string]uint64{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 {
			values[strings.TrimSuffix(fields[0], ":")], _ = strconv.ParseUint(fields[1], 10, 64)
		}
	}
	total := values["MemTotal"] * 1024
	available := values["MemAvailable"] * 1024
	if available > total {
		available = total
	}
	return total - available, total, scanner.Err()
}

func readFirstFloat(path string) (float64, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(payload))
	if len(fields) == 0 {
		return 0, errors.New("metric file is empty")
	}
	return strconv.ParseFloat(fields[0], 64)
}

func platform() string {
	release, _ := os.ReadFile("/proc/sys/kernel/osrelease")
	return strings.TrimSpace(fmt.Sprintf("%s %s %s", runtime.GOOS, strings.TrimSpace(string(release)), runtime.GOARCH))
}

func env(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func round1(value float64) float64 { return float64(int(value*10+0.5)) / 10 }
