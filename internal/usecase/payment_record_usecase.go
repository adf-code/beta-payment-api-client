package usecase

import (
	"beta-payment-api-client/internal/contextkeys"
	"beta-payment-api-client/internal/entity"
	"beta-payment-api-client/internal/repository"
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"math/rand"
	"strings"
	"sync"
	"time"
)

var seen sync.Map

type PaymentRecordUseCase interface {
	StartPolling(ctx context.Context, id uuid.UUID) error
	StartConsumer(ctx context.Context) error
	BoostOtherTasks(id uuid.UUID) error
	Create(ctx context.Context, paymentRecord entity.PaymentRecord) (*entity.PaymentRecord, error)
	GetByID(ctx context.Context, id uuid.UUID) (*entity.PaymentRecord, error)
	ListRunningTasks() []uuid.UUID
	RestorePollingTasks(ctx context.Context) error
	DebugDumpTasks()
}

type taskHandle struct {
	ctx    context.Context
	cancel context.CancelFunc
	wake   chan struct{} // sinyal boost/reset delay
}

type paymentRecordUseCase struct {
	paymentRecordRepo         repository.PaymentRecordRepository
	paymentRecordCheckLogRepo repository.PaymentRecordCheckLogRepository
	tasks                     sync.Map
	db                        *sql.DB
	logger                    zerolog.Logger
}

func NewPaymentRecordUseCase(
	paymentRecordRepo repository.PaymentRecordRepository,
	paymentRecordCheckLogRepo repository.PaymentRecordCheckLogRepository,
	db *sql.DB,
	logger zerolog.Logger) PaymentRecordUseCase {
	return &paymentRecordUseCase{
		paymentRecordRepo:         paymentRecordRepo,
		paymentRecordCheckLogRepo: paymentRecordCheckLogRepo,
		db:                        db,
		logger:                    logger,
	}
}

func (paymentRecordUC *paymentRecordUseCase) StartPollingYangLama(ctx context.Context, id uuid.UUID) error {
	go func() {
		delay := 10 * time.Second
		maxDelay := 60 * time.Second

		for {
			select {
			case <-ctx.Done():
				return
			default:
				paymentRecordUC.logger.Info().Msgf("⚓️ Polling Payment Record with id: %s", id)
				status, paymentRecordCheckHTTP, err := paymentRecordUC.paymentRecordRepo.FetchPaymentStatus(
					context.WithValue(ctx, contextkeys.CtxKeyPollingDelay, delay),
					id,
				)

				err = paymentRecordUC.paymentRecordCheckLogRepo.LogFetchAttempt(paymentRecordCheckHTTP, delay)

				if err != nil {
					paymentRecordUC.logger.Error().Msgf("❌ Error fetching Payment Record with: %s", err)
				}

				if status == "PAID" || status == "UNPAID" {
					paymentRecordUC.logger.Info().Msgf("️🔄 Finalized: %s -> %s", id, status)
					_ = paymentRecordUC.paymentRecordRepo.PublishSuccessEvent(ctx, id)

					// ✅ Remove from in-memory tasks
					paymentRecordUC.tasks.Delete(id)

					// ✅ Remove from Redis persistence
					_ = paymentRecordUC.paymentRecordRepo.RemovePollingTask(ctx, id)
					return
				}

				_ = paymentRecordUC.paymentRecordRepo.SetNextRetry(ctx, id, delay)
				time.Sleep(delay)

				if delay < maxDelay {
					delay *= 2
				}
			}
		}
	}()
	paymentRecordUC.tasks.Store(id, true)
	_ = paymentRecordUC.paymentRecordRepo.PersistPollingTask(ctx, id)
	paymentRecordUC.DebugDumpTasks()
	return nil
}

func (paymentRecordUC *paymentRecordUseCase) StartPolling(ctx context.Context, id uuid.UUID) error {
	paymentRecordUC.logger.Info().Msgf("[StartPolling] paymentRecordUC=%p id=%s", paymentRecordUC, id)
	key := id.String()

	// Cegah worker ganda
	if _, loaded := paymentRecordUC.tasks.Load(key); loaded {
		paymentRecordUC.logger.Info().Msgf("⏭️ Task already running: %s", id)
		return nil
	}

	// Buat handle + simpan
	wctx, cancel := context.WithCancel(ctx)
	h := &taskHandle{
		ctx:    wctx,
		cancel: cancel,
		wake:   make(chan struct{}, 1), // buffered agar non-blocking
	}
	paymentRecordUC.tasks.Store(key, h)

	// Persist marker aktif (opsional)
	_ = paymentRecordUC.paymentRecordRepo.PersistPollingTask(ctx, id)

	// Mulai worker
	go paymentRecordUC.pollWorker(h, id)

	return nil
}

func (paymentRecordUC *paymentRecordUseCase) pollWorker(h *taskHandle, id uuid.UUID) {
	key := id.String()
	delay := 10 * time.Second
	maxDelay := 80 * time.Second // sesuai ekspektasi kamu

	for {
		// 1) Cek sekarang
		paymentRecordUC.logger.Info().Msgf("⚓️ Polling Payment Record with id: %s", id)

		status, paymentRecordCheckHTTP, fetchErr := paymentRecordUC.paymentRecordRepo.FetchPaymentStatus(
			context.WithValue(h.ctx, contextkeys.CtxKeyPollingDelay, delay),
			id,
		)

		if logErr := paymentRecordUC.paymentRecordCheckLogRepo.LogFetchAttempt(paymentRecordCheckHTTP, delay); logErr != nil {
			paymentRecordUC.logger.Error().Msgf("❌ LogFetchAttempt error: %v", logErr)
		}
		if fetchErr != nil {
			paymentRecordUC.logger.Error().Msgf("❌ FetchPaymentStatus error: %v", fetchErr)
		}

		// 2) Final?
		if status == "PAID" || status == "UNPAID" {
			paymentRecordUC.logger.Info().Msgf("️🔄 Finalized: %s -> %s", id, status)
			_ = paymentRecordUC.paymentRecordRepo.PublishSuccessEvent(h.ctx, id)

			// ❗❗ PENTING: JANGAN panggil BoostOtherTasks di sini.
			// Biarkan Kafka consumer yang melakukan boost agar tidak double.

			// Cleanup
			paymentRecordUC.tasks.Delete(key)
			_ = paymentRecordUC.paymentRecordRepo.RemovePollingTask(h.ctx, id)
			h.cancel()
			return
		}

		// 3) Simpan informasi next retry (opsional)
		_ = paymentRecordUC.paymentRecordRepo.SetNextRetry(h.ctx, id, delay)

		// 4) Tunggu dengan timer yang bisa di-reset
		timer := time.NewTimer(delay)
		select {
		case <-h.ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			paymentRecordUC.tasks.Delete(key)
			return

		case <-h.wake:
			// BOOST: stop timer lama, reset delay ke 10s, cek ulang
			if !timer.Stop() {
				<-timer.C
			}
			delay = 10 * time.Second
			continue

		case <-timer.C:
			// timeout normal → exponential backoff
			if delay < maxDelay {
				delay *= 2
				if delay > maxDelay {
					delay = maxDelay
				}
			}
			// loop lagi → cek lagi
		}
	}
}

func (paymentRecordUC *paymentRecordUseCase) BoostOtherTasksYangLama(id uuid.UUID) error {
	paymentRecordUC.tasks.Range(func(key, _ interface{}) bool {
		paymentID := key.(uuid.UUID)
		if paymentID != id {
			go paymentRecordUC.StartPolling(context.Background(), paymentID)
		}
		return true
	})
	return nil
}

func (paymentRecordUC *paymentRecordUseCase) BoostOtherTasks(successID uuid.UUID) error {
	successKey := successID.String()

	paymentRecordUC.tasks.Range(func(k, v any) bool {
		key, ok := k.(string)
		if !ok || key == successKey {
			return true
		}
		h, ok := v.(*taskHandle)
		if !ok || h == nil {
			return true
		}
		// kirim sinyal non-blocking; jika sudah ada sinyal pending, skip
		select {
		case h.wake <- struct{}{}:
			paymentRecordUC.logger.Info().Msgf("🚀 Boosted task %s (reset delay to 10s & immediate check)", key)
		default:
		}
		return true
	})
	return nil
}

func (u *paymentRecordUseCase) StartConsumer(ctx context.Context) error {
	go func() {
		backoff := 500 * time.Millisecond
		maxBackoff := 5 * time.Second

		for {
			select {
			case <-ctx.Done():
				u.logger.Info().Msg("⁉️ Kafka consumer stopped")
				return
			default:
			}

			// Read with a short deadline so loop stays responsive
			readCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			paymentIDStr, err := u.paymentRecordRepo.ReadKafkaMessage(readCtx)
			cancel()

			if err != nil {
				// Treat broker fetch timeouts / no data as benign
				if errors.Is(err, context.DeadlineExceeded) ||
					strings.Contains(strings.ToLower(err.Error()), "request timed out") ||
					strings.Contains(strings.ToLower(err.Error()), "no messages") {
					u.logger.Debug().Msgf("[Kafka] idle/no messages (will keep polling): %v", err)
					// reset backoff on benign idle
					backoff = 500 * time.Millisecond
					continue
				}

				// For other transient errors, warn + backoff (with cap)
				u.logger.Warn().Msgf("[Kafka] transient error: %v", err)
				time.Sleep(backoff + time.Duration(rand.Intn(250))*time.Millisecond)
				if backoff < maxBackoff {
					backoff *= 2
					if backoff > maxBackoff {
						backoff = maxBackoff
					}
				}
				continue
			}

			backoff = 500 * time.Millisecond

			// Parse UUID
			paymentID, parseErr := uuid.Parse(paymentIDStr)
			if parseErr != nil {
				u.logger.Warn().Msgf("❌ Invalid UUID from Kafka: %q (%v)", paymentIDStr, parseErr)
				continue
			}

			if _, loaded := seen.LoadOrStore(paymentID.String(), true); loaded {
				u.logger.Info().Msgf("⁉️ Duplicate message ignored: %s", paymentID)
				continue
			}

			u.logger.Info().Msgf("⚡ Boost triggered by: %s", paymentID)
			_ = u.BoostOtherTasks(paymentID)
			u.DebugDumpTasks()
		}
	}()
	return nil
}

func (paymentRecordUC *paymentRecordUseCase) Create(ctx context.Context, paymentRecord entity.PaymentRecord) (*entity.PaymentRecord, error) {
	paymentRecordUC.logger.Info().Str("usecase", "Create").Msg("⚙️ Store payment records")
	tx, err := paymentRecordUC.db.Begin()
	if err != nil {
		paymentRecordUC.logger.Error().Err(err).Msg("❌ Failed to begin transaction")
		return nil, err
	}

	err = paymentRecordUC.paymentRecordRepo.Store(ctx, tx, &paymentRecord)
	if err != nil {
		tx.Rollback()
		paymentRecordUC.logger.Error().Err(err).Msg("❌ Failed to store payment records, rolling back")
		return nil, err
	}

	err = tx.Commit()
	if err != nil {
		paymentRecordUC.logger.Error().Err(err).Msg("❌ Failed to commit transaction")
		return nil, err
	}

	paymentRecordUC.logger.Info().Str("payment_id", paymentRecord.ID.String()).Msg("✅ Payment records created")
	return &paymentRecord, nil
}

func (paymentRecordUC *paymentRecordUseCase) GetByID(ctx context.Context, id uuid.UUID) (*entity.PaymentRecord, error) {
	paymentRecordUC.logger.Info().Str("usecase", "GetByID").Msg("⚙️ Fetching payment records by ID")
	return paymentRecordUC.paymentRecordRepo.FetchByID(ctx, id)
}

func (paymentRecordUC *paymentRecordUseCase) ListRunningTasksYangLama() []uuid.UUID {
	var ids []uuid.UUID
	paymentRecordUC.tasks.Range(func(key, value any) bool {
		if id, ok := key.(uuid.UUID); ok {
			ids = append(ids, id)
		}
		return true
	})
	return ids
}

func (paymentRecordUC *paymentRecordUseCase) ListRunningTasks() []uuid.UUID {
	var ids []uuid.UUID
	paymentRecordUC.tasks.Range(func(k, _ any) bool {
		switch t := k.(type) {
		case string:
			if id, err := uuid.Parse(t); err == nil {
				ids = append(ids, id)
			}
		case uuid.UUID:
			ids = append(ids, t)
		}
		return true
	})
	return ids
}

func (paymentRecordUC *paymentRecordUseCase) RestorePollingTasks(ctx context.Context) error {
	ids, err := paymentRecordUC.paymentRecordRepo.RestorePollingTasks(ctx)
	if err != nil {
		paymentRecordUC.logger.Error().Err(err).Msg("❌ Failed to restore polling tasks from Redis")
		return err
	}

	for _, id := range ids {
		paymentRecordUC.logger.Info().Msgf("♻️ Restoring polling task: %s", id)
		_ = paymentRecordUC.StartPolling(ctx, id)
	}
	return nil
}

func (paymentRecordUC *paymentRecordUseCase) DebugDumpTasks() {
	paymentRecordUC.tasks.Range(func(k, v any) bool {
		paymentRecordUC.logger.Info().Msgf("[tasks] key=%v typeV=%T", k, v)
		return true
	})
}
