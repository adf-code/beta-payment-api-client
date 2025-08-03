package payment_record

import (
	"beta-payment-api-client/internal/delivery/response"
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
	p.Logger.Info().Msg("📥 Incoming GetByID request")
	response.Success(w, 200, "paymentRecords", "checkPaymentRecordByID", "Success Check Payment Record by ID", nil)
}
