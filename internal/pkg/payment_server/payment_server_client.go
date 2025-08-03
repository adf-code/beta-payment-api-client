package payment_server

import (
	"beta-payment-api-client/config"
	"fmt"
	"github.com/rs/zerolog"
	"net/http"
)

type PaymentServerClient struct {
	baseURL string
	apiKey  string
	logger  zerolog.Logger
}

func NewPaymentServerClient(cfg *config.AppConfig, logger zerolog.Logger) *PaymentServerClient {
	return &PaymentServerClient{
		baseURL: cfg.PaymentServerBaseURL,
		apiKey:  cfg.PaymentServerAPIKey,
		logger:  logger,
	}
}

func (p *PaymentServerClient) InitPaymentServer() error {
	url := fmt.Sprintf("%s/healthz", p.baseURL)

	resp, err := http.Get(url)
	if err != nil {
		p.logger.Error().Err(err).Msgf("❌ Error connecting to Payment Server: %v", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("unexpected status code: %d", resp.StatusCode)
		p.logger.Error().Err(err).Msg("❌ Failed to connect to Payment Server")
		return err
	}

	p.logger.Info().Msgf("✅ Payment Server is healthy at %s", url)
	return nil
}
