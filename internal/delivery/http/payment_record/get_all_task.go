package payment_record

import (
	"beta-payment-api-client/internal/delivery/response"
	"net/http"
)

func (p *PaymentRecordHandler) GetAllTask(w http.ResponseWriter, r *http.Request) {
	p.Logger.Info().Msg("📥 Incoming GetAllTask request")
	runningTasks := p.PaymentRecordUC.ListRunningTasks()
	p.Logger.Info().Int("count", len(runningTasks)).Msg("✅ Successfully fetched payments")
	response.Success(w, 200, "payment_records", "GetAllTask", "Success Get All Tasks", runningTasks)
}
