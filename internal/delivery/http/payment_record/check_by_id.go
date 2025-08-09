package payment_record

import (
	"beta-payment-api-client/internal/delivery/request"
	"beta-payment-api-client/internal/delivery/response"
	"beta-payment-api-client/internal/entity"
	"beta-payment-api-client/internal/valueobject"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"math/big"
	"net/http"
)

// CheckByID godoc
// @Summary      Check payment record by ID
// @Description  Retrieve a payment record entity using its UUID
// @Tags         payment_records
// @Security     BearerAuth
// @Param        id   path      string  true  "UUID of the payment record"
// @Success      200  {object}  response.APIResponse
// @Failure      400  {object}  response.APIResponse  "Invalid UUID"
// @Failure      401  {object}  response.APIResponse  "Unauthorized"
// @Failure      404  {object}  response.APIResponse  "Book not found"
// @Failure      500  {object}  response.APIResponse  "Internal server error"
// @Router       /api/v1/payment-records/check/{id} [get]
func (p *PaymentRecordHandler) CheckByID(w http.ResponseWriter, r *http.Request) {
	p.Logger.Info().Msg("📥 Incoming CheckByID request")

	var req request.CheckPaymentRecord
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		p.Logger.Error().Err(err).Msg("❌ Failed to decode request body")
		response.Failed(w, 400, "paymentRecords", "checkPaymentRecordByID", "Invalid Request Body")
		return
	}

	if err := req.Validate(); err != nil {
		p.Logger.Error().Err(err).Msg("❌ Validation error")
		response.Failed(w, 422, "paymentRecords", "checkPaymentRecordByID", "Validation Error")
		return
	}

	id, err := uuid.Parse(req.ID)
	if err != nil {
		p.Logger.Error().Err(err).Msg("❌ Invalid UUID parameter")
		response.Failed(w, 422, "payments", "updatePaymentByID", "Invalid UUID")
		return
	}

	paymentRecord, err := p.PaymentRecordUC.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {

			zero := valueobject.BigFloat{Float: big.NewFloat(0)}

			paymentRecordCreate := entity.PaymentRecord{
				ID:          id,
				Tag:         "",
				Description: "",
				Amount:      zero,
				Status:      "",
			}

			newPayment, err := p.PaymentRecordUC.Create(r.Context(), paymentRecordCreate)
			if err != nil {
				p.Logger.Error().Err(err).Msg("❌ Failed to store payment, general")
				response.Failed(w, 500, "paymentRecords", "checkPaymentRecordByID", "Error Create Payment")
				return
			}
			_ = p.PaymentRecordUC.StartPolling(context.Background(), id)
			p.Logger.Info().Str("data", fmt.Sprint(newPayment)).Msg("✅ Successfully stored payment")
			response.Success(w, 200, "paymentRecords", "checkPaymentRecordByID", "Success Check Payment Record by ID", newPayment)
			return
		}
		p.Logger.Error().Err(err).Msg("❌ Failed to get payment by ID, general")
		response.Failed(w, 500, "paymentRecords", "checkPaymentRecordByID", "Error Get Payment by ID")
		return
	}
	_ = p.PaymentRecordUC.StartPolling(context.Background(), id)
	p.Logger.Info().Str("data", fmt.Sprint(paymentRecord.ID)).Msg("✅ Successfully get payment by id")
	response.Success(w, 200, "paymentRecords", "checkPaymentRecordByID", "Success Get Payment by ID", paymentRecord)
}
