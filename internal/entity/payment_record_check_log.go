package entity

import (
	"encoding/json"
	"github.com/google/uuid"
	"time"
)

type PaymentRecordCheckLog struct {
	ID              uuid.UUID       `json:"id"`
	PaymentID       uuid.UUID       `json:"payment_id"`
	OccurredAt      *time.Time      `json:"occurred_at"`
	Method          string          `json:"method"`
	URL             string          `json:"url"`
	RequestHeaders  json.RawMessage `json:"request_headers"`
	RequestBody     []byte          `json:"request_body"`
	ResponseHeaders json.RawMessage `json:"response_headers"`
	ResponseBody    []byte          `json:"response_body"`
	StatusCode      int             `json:"status_code"`
	DelaySeconds    int64           `json:"delay_seconds"`
	CreatedAt       *time.Time      `json:"created_at"`
	UpdatedAt       *time.Time      `json:"updated_at"`
}
