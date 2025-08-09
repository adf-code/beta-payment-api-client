package repository

import (
	"beta-payment-api-client/internal/entity"
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"net/http"
	"time"
)

type PaymentRecordCheckLogRepository interface {
	Store(ctx context.Context, tx *sql.Tx, payment *entity.PaymentRecordCheckLog) error
	LogFetchAttempt(paymentRecordCheckHTTP *entity.PaymentRecordCheckHTTP, delaySeconds time.Duration) error
}

type paymentRecordCheckLogRepo struct {
	DB     *sql.DB
	logger zerolog.Logger
}

func NewPaymentRecordCheckLogRepository(
	db *sql.DB,
	logger zerolog.Logger) PaymentRecordCheckLogRepository {
	return &paymentRecordCheckLogRepo{
		DB:     db,
		logger: logger,
	}
}

func (p *paymentRecordCheckLogRepo) Store(ctx context.Context, tx *sql.Tx, paymentRecordCheckLog *entity.PaymentRecordCheckLog) error {
	return tx.QueryRowContext(
		ctx,
		"INSERT INTO payment_record_check_logs ("+
			"payment_id, method, url, request_headers, request_body, response_headers, response_body, status_code, delay_seconds) "+
			"VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id, occurred_at, created_at, updated_at",
		paymentRecordCheckLog.PaymentID, paymentRecordCheckLog.Method, paymentRecordCheckLog.URL,
		paymentRecordCheckLog.RequestHeaders, paymentRecordCheckLog.RequestBody, paymentRecordCheckLog.ResponseHeaders,
		paymentRecordCheckLog.ResponseBody, paymentRecordCheckLog.StatusCode, paymentRecordCheckLog.DelaySeconds,
	).Scan(&paymentRecordCheckLog.ID, &paymentRecordCheckLog.OccurredAt, &paymentRecordCheckLog.CreatedAt, &paymentRecordCheckLog.UpdatedAt)
}

func (p *paymentRecordCheckLogRepo) LogFetchAttempt(
	paymentRecordCheckHTTP *entity.PaymentRecordCheckHTTP,
	delay time.Duration,
) error {
	// Guard repo & argumen
	if p == nil || p.DB == nil {
		return errors.New("log repo/db is nil")
	}
	if paymentRecordCheckHTTP == nil {
		return errors.New("paymentRecordCheckHTTP is nil")
	}

	// ===== Request (aman dari nil) =====
	var (
		method     string
		urlStr     string
		reqHeaders http.Header
	)
	if paymentRecordCheckHTTP.Request != nil {
		method = paymentRecordCheckHTTP.Request.Method
		if paymentRecordCheckHTTP.Request.URL != nil {
			urlStr = paymentRecordCheckHTTP.Request.URL.String()
		}
		reqHeaders = paymentRecordCheckHTTP.Request.Header // http.Header (map) — boleh nil
	}

	reqHeadersRedacted := redactHeaders(reqHeaders) // aman walau nil
	reqHeadersJSON, _ := marshalHeaders(reqHeadersRedacted)

	// GET → biasanya tidak ada body; kalau mau capture POST/PUT nanti pakai req.GetBody()
	var reqBody []byte = nil

	// ===== Response (aman dari nil) =====
	var (
		respHeaders     http.Header
		respHeadersJSON []byte
	)
	if paymentRecordCheckHTTP.Response != nil {
		respHeaders = paymentRecordCheckHTTP.Response.Header // boleh nil
	}
	// Walau ResponseBody nil/non-nil, header tetap kita simpan apa adanya (bisa kosong)
	respHeadersRedacted := redactHeaders(respHeaders)
	respHeadersJSON, _ = marshalHeaders(respHeadersRedacted)

	// ===== Delay detik (bukan nanoseconds) =====
	delaySeconds := int64(delay.Seconds())
	if delaySeconds < 0 {
		delaySeconds = 0
	}

	// ===== Bangun row =====
	logRow := entity.PaymentRecordCheckLog{
		ID:              uuid.New(),
		PaymentID:       paymentRecordCheckHTTP.ID,
		Method:          method,
		URL:             urlStr,
		RequestHeaders:  reqHeadersJSON,
		RequestBody:     reqBody,
		ResponseHeaders: respHeadersJSON,
		ResponseBody:    paymentRecordCheckHTTP.ResponseBody, // boleh nil
		StatusCode:      paymentRecordCheckHTTP.StatusCode,   // 0 jika belum ada resp
		DelaySeconds:    delaySeconds,
	}

	// ===== Transaksi (pakai context yang ada) =====
	ctx := context.Background()
	if paymentRecordCheckHTTP.Context != nil {
		ctx = paymentRecordCheckHTTP.Context
	}

	tx, err := p.DB.Begin()
	if err != nil {
		p.logger.Error().Err(err).Msg("❌ Failed to begin transaction")
		return err
	}
	defer func() {
		// Pastikan rollback bila belum commit
		_ = tx.Rollback()
	}()

	if err := p.Store(ctx, tx, &logRow); err != nil {
		p.logger.Error().Err(err).Msg("❌ Failed to store payment record log")
		return err
	}

	if err := tx.Commit(); err != nil {
		p.logger.Error().Err(err).Msg("❌ Failed to commit transaction")
		return err
	}
	return nil
}
