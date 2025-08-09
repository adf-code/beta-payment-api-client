package entity

import (
	"context"
	"github.com/google/uuid"
	"net/http"
)

type PaymentRecordCheckHTTP struct {
	Context      context.Context
	ID           uuid.UUID
	Request      *http.Request
	Response     *http.Response
	ResponseBody []byte
	StatusCode   int
}
