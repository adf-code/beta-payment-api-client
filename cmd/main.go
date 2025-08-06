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
	"beta-payment-api-client/internal/repository"
	"github.com/google/uuid"
	"log"

	pkgKafka "beta-payment-api-client/internal/pkg/kafka"
	pkgLogger "beta-payment-api-client/internal/pkg/logger"
	pkgPaymentServer "beta-payment-api-client/internal/pkg/payment_server"
	pkgRedis "beta-payment-api-client/internal/pkg/redis"
	//"beta-payment-api-client/internal/repository"
	//"beta-payment-api-client/internal/usecase"
	"context"
	"database/sql"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
)

func main() {
	_ = godotenv.Load() // Load .env

	// Load env config
	cfg := config.LoadConfig()

	logger := pkgLogger.InitLoggerWithTelemetry(cfg)

	//postgresClient := pkgDatabase.NewPostgresClient(cfg, logger)
	//db := postgresClient.InitPostgresDB()

	redisClient := pkgRedis.NewRedisClient(cfg, logger)
	redis := redisClient.InitRedis()

	kafkaProductClient := pkgKafka.NewKafkaProducerClient(cfg, logger)
	kafkaProducer := kafkaProductClient.InitKafkaProducer()

	kafkaConsumerClient := pkgKafka.NewKafkaConsumerClient(cfg, logger)
	kafkaConsumer := kafkaConsumerClient.InitKafkaConsumer()

	paymentServerClient := pkgPaymentServer.NewPaymentServerClient(cfg, logger)
	err := paymentServerClient.InitPaymentServer()
	if err != nil {
		logger.Fatal().Err(err).Msgf("❌ Error to connect to Payment Server: %v", err)
	}

	paymentIDs := []string{"73f51a05-188e-4fac-ad6c-f806dca5da6d", "a9736df9-8874-4207-b0ad-401957a6aee1", "ead91c6e-c72a-484c-95dc-2f8067c06ec1"}

	paymentRecordRepo := repository.NewPaymentRecordRepository(redis, kafkaProducer, kafkaConsumer, cfg.PaymentServerAPIKey, cfg.KafkaTopicPaymentSuccess)
	_ = paymentRecordRepo.StartConsumer(context.Background())

	for _, idStr := range paymentIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			log.Println("invalid UUID:", idStr, "error:", err)
			continue
		}

		err = paymentRecordRepo.StartPolling(context.Background(), id)
		if err != nil {
			log.Println("StartPolling error for", id, ":", err)
		}
	}

	select {} // block forever

	//======

	//// Repository and HTTP handler
	//paymentRecordRepo := repository.NewPaymentRecordRepository(redis)
	//paymentRecordUC := usecase.NewPaymentRecordUsecase(paymentRecordRepo)
	//handler := deliveryHttp.SetupHandler(paymentRecordUC, logger)
	//
	//// HTTP server config
	//server := &http.Server{
	//	Addr:    fmt.Sprintf(":%s", cfg.Port),
	//	Handler: handler,
	//}
	//
	//// Run server in goroutine
	//go func() {
	//	logger.Info().Msgf("🟢 Server running on http://localhost:%s", cfg.Port)
	//	logger.Info().Msgf("📚 Swagger running on http://localhost:%s/swagger/index.html", cfg.Port)
	//	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
	//		logger.Fatal().Err(err).Msgf("❌ Server failed: %v", err)
	//	}
	//}()
	//
	//// Setup signal listener
	//quit := make(chan os.Signal, 1)
	//signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	//<-quit
	//
	//logger.Info().Msgf("🛑 Gracefully shutting down server...")
	//
	//// Graceful shutdown context
	//ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	//defer cancel()
	//
	//// Shutdown HTTP server
	//if err := server.Shutdown(ctx); err != nil {
	//	logger.Fatal().Err(err).Msgf("❌ Server shutdown failed: %v", err)
	//}
	//
	//// ✅ Close PostgreSQL DB
	//closePostgres(db, logger)
	//
	//logger.Info().Msgf("✅ Server shutdown completed.")
}

func closePostgres(db *sql.DB, logger zerolog.Logger) {
	if err := db.Close(); err != nil {
		logger.Info().Msgf("⚠️ Failed to close PostgreSQL connection: %v", err)
	} else {
		logger.Info().Msgf("🔒 PostgreSQL connection closed.")
	}
}
