package entity

import (
	"beta-payment-api-client/internal/valueobject"
	"github.com/google/uuid"
	"time"
)

type PaymentStatus string

type PaymentRecord struct {
	ID          uuid.UUID            `json:"id"`
	Tag         string               `json:"tag"`
	Description string               `json:"description"`
	Amount      valueobject.BigFloat `json:"amount"`
	Status      PaymentStatus        `json:"status"`
	CreatedAt   *time.Time           `json:"created_at"`
	UpdatedAt   *time.Time           `json:"updated_at"`
}
