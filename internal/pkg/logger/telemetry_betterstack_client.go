package logger

import (
	"beta-payment-api-client/config"
	"bytes"
	"github.com/rs/zerolog"
	"net/http"
	"os"
	"time"
)

type TelemetryClient struct {
	client   *http.Client
	apiKey   string
	endpoint string
}

func NewTelemetryClient(apiKey string, endpoint string) *TelemetryClient {
	return &TelemetryClient{
		client:   &http.Client{Timeout: 5 * time.Second},
		apiKey:   apiKey,
		endpoint: endpoint,
	}
}

func InitLoggerWithTelemetry(cfg *config.AppConfig) zerolog.Logger {
	if cfg.TelemetryAPIKey == "" || cfg.TelemetryEndpoint == "" {
		panic("Telemetry config is not set")
	}

	consoleWriter := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	telemetryWriter := NewTelemetryClient(cfg.TelemetryAPIKey, cfg.TelemetryEndpoint)
	multiWriter := zerolog.MultiLevelWriter(consoleWriter, telemetryWriter)

	return zerolog.New(multiWriter).With().Timestamp().Logger()
}

func (t *TelemetryClient) Write(p []byte) (n int, err error) {
	req, err := http.NewRequest("POST", t.endpoint, bytes.NewBuffer(p))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.apiKey)

	resp, err := t.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	return len(p), nil
}
