package payment_record

import (
	"beta-payment-api-client/internal/usecase"
	"github.com/rs/zerolog"
)

type PaymentRecordHandler struct {
	PaymentRecordUC usecase.PaymentRecordUsecase
	Logger          zerolog.Logger
	//EmailClient *mail.SendGridClient
}

func NewPaymentRecordHandler(paymentRecord usecase.PaymentRecordUseCase, logger zerolog.Logger) *BookHandler {
	return &PaymentRecordHandler{PaymentRecordUC: paymentRecord, Logger: logger}
}
