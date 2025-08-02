package usecase

import (
	"beta-payment-api-client/internal/dto"
	"beta-payment-api-client/internal/entity"
	"beta-payment-api-client/internal/repository"
	"beta-payment-api-client/internal/valueobject"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"math/big"
	"net/http"
)

type PaymentRecordUsecase interface {
	Check(paymentID string) (*entity.PaymentRecord, error)
}

type paymentRecordUsecase struct {
	repo repository.PaymentRecordRepository
}

func NewPaymentCheckUsecase(repo repository.PaymentRecordRepository) PaymentRecordUsecase {
	return &paymentRecordUsecase{repo: repo}
}

func (uc *paymentRecordUsecase) Check(paymentID string) (*entity.PaymentRecord, error) {
	url := fmt.Sprintf("http://localhost:8080/api/v1/payments/%s", paymentID)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var body dto.GetPaymentByIDResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}

	idParsed, _ := uuid.Parse(body.Data.ID)
	result := &entity.PaymentRecord{
		ID:        idParsed,
		Tag:       body.Data.Tag,
		Amount:    valueobject.BigFloat{big.NewFloat(body.Data.Amount)},
		Status:    entity.PaymentStatus(body.Data.Status),
		CreatedAt: &body.Data.CreatedAt,
		UpdatedAt: &body.Data.UpdatedAt,
	}

	if err := uc.repo.Store(paymentID, *result); err != nil {
		return nil, err
	}

	return result, nil
}
