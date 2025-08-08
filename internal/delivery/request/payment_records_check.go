package request

import (
	"errors"
)

type CheckPaymentRecord struct {
	ID string `json:"id"`
}

func (r *CheckPaymentRecord) Validate() error {
	if r.ID == "" {
		return errors.New("id is required")
	}
	return nil
}
