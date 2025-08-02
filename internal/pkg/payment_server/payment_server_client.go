package payment_server

import (
	"beta-payment-api-client/config"
	"fmt"
	"github.com/rs/zerolog"
	"net/http"
)

type PaymentServerClient struct {
	baseURL string
	bearer  string
	logger  zerolog.Logger
}

func NewPaymentServerClient(cfg *config.AppConfig, logger zerolog.Logger) *PaymentServerClient {
	return &PaymentServerClient{
		baseURL: cfg.RedisHost,
		bearer:  cfg.RedisPort,
		logger:  logger,
	}
}

func (p *PaymentServerClient) InitPaymentServer() {
	url := fmt.Sprintf("%s/healtz", p.baseURL)

	resp, err := http.Get(url)
	if err != nil {
		p.logger.Fatal().Err(err).Msgf("❌ Error to connect to Payment Server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		p.logger.Fatal().Err(err).Msgf("❌ Failed to connect to Payment Server: %v", err)
	}
}
