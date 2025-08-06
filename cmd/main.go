// @title           Beta Book API
// @version         1.0
// @description     API service to manage books using Clean Architecture

// @contact.name   ADF Code
// @contact.url    https://github.com/adf-code

// @host      localhost:8080

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Using token header using the Bearer scheme. Example: "Bearer {token}"

package main

import (
	"beta-payment-api-client/config"
	_ "beta-payment-api-client/docs"
	pkgKafka "beta-payment-api-client/internal/pkg/kafka"
	pkgLogger "beta-payment-api-client/internal/pkg/logger"
	pkgPaymentServer "beta-payment-api-client/internal/pkg/payment_server"
	pkgRedis "beta-payment-api-client/internal/pkg/redis"
	"beta-payment-api-client/internal/repository"
	"beta-payment-api-client/internal/usecase"
	"context"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	cfg := config.LoadConfig()
	logger := pkgLogger.InitLoggerWithTelemetry(cfg)

	redisClient := pkgRedis.NewRedisClient(cfg, logger).InitRedis()
	kafkaProducer := pkgKafka.NewKafkaProducerClient(cfg, logger).InitKafkaProducer()
	kafkaConsumer := pkgKafka.NewKafkaConsumerClient(cfg, logger).InitKafkaConsumer()
	paymentServerClient := pkgPaymentServer.NewPaymentServerClient(cfg, logger)

	err := paymentServerClient.InitPaymentServer()
	if err != nil {
		logger.Fatal().Err(err).Msg("Cannot reach payment server")
	}

	repo := repository.NewPaymentRecordRepository(redisClient, kafkaProducer, kafkaConsumer, cfg.PaymentServerAPIKey, cfg.KafkaTopicPaymentSuccess)
	paymentRecordUC := usecase.NewPaymentRecordUseCase(repo)

	// Start Kafka consumer
	_ = paymentRecordUC.StartConsumer(context.Background())

	// Start polling manually for testing
	paymentIDs := []string{"73f51a05-188e-4fac-ad6c-f806dca5da6d", "a9736df9-8874-4207-b0ad-401957a6aee1", "ead91c6e-c72a-484c-95dc-2f8067c06ec1"}
	for _, idStr := range paymentIDs {
		id, err := uuid.Parse(idStr)
		if err == nil {
			_ = paymentRecordUC.StartPolling(context.Background(), id)
		}
	}

	select {} // block
}
