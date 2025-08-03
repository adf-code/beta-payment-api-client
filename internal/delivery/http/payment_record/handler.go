package payment_record

import (
	"beta-payment-api-client/internal/usecase"
	"github.com/rs/zerolog"
)

type PaymentRecordHandler struct {
	PaymentRecordUC usecase.PaymentRecordUseCase
	Logger          zerolog.Logger
	//EmailClient *mail.SendGridClient
}

func NewPaymentRecordHandler(paymentRecord usecase.PaymentRecordUseCase, logger zerolog.Logger) *PaymentRecordHandler {
	return &PaymentRecordHandler{PaymentRecordUC: paymentRecord, Logger: logger}
}
