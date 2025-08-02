package dto

import "time"

type GetPaymentByIDResponse struct {
	Status  string       `json:"status"`
	Entity  string       `json:"entity"`
	State   string       `json:"state"`
	Message string       `json:"message"`
	Data    PaymentData  `json:"data"`
}

type PaymentData struct {
	ID          string    `json:"id"`
	Tag         string    `json:"tag"`
	Description string    `json:"description"`
	Amount      int64     `json:"amount"`
	Status      string    `json:"status"` // Expects values like "PENDING", "PAID"
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
