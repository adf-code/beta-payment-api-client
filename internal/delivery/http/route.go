package http

import (
	"beta-payment-api-client/internal/delivery/http/health"
	"beta-payment-api-client/internal/delivery/http/middleware"
	"beta-payment-api-client/internal/delivery/http/payment_record"
	"beta-payment-api-client/internal/delivery/http/router"
	"beta-payment-api-client/internal/usecase"
	"github.com/rs/zerolog"

	"github.com/swaggo/http-swagger"
	"net/http"
)

func SetupHandler(paymentRecordUC usecase.PaymentRecordUseCase, logger zerolog.Logger) http.Handler {
	paymentRecordHandler := payment_record.PaymentRecordHandler{paymentRecordUC, logger}
	healthHandler := health.HealthHandler{logger}
	auth := middleware.AuthMiddleware(logger)
	log := middleware.LoggingMiddleware(logger)

	r := router.NewRouter()

	r.HandlePrefix(http.MethodGet, "/swagger/", httpSwagger.WrapHandler)

	r.Handle("GET", "/healthz", middleware.Chain(log)(healthHandler.Check))

	//r.Handle("GET", "/api/v1/payment-records/check/histories/{id}", middleware.Chain(log, auth)(paymentRecordHandler.))
	r.Handle("GET", "/api/v1/payment-records/check/{id}", middleware.Chain(log, auth)(paymentRecordHandler.CheckByID))

	return r
}
