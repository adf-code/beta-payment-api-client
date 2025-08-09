CREATE EXTENSION IF NOT EXISTS "pgcrypto";
CREATE TABLE IF NOT EXISTS payment_record_check_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid (),
    payment_id UUID NOT NULL,
    occurred_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    method TEXT NOT NULL,
    url TEXT NOT NULL,
    request_headers JSONB,
    request_body BYTEA,
    response_headers JSONB,
    response_body BYTEA,
    status_code INT,
    delay_seconds INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP
    );

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_payment_record_check_logs_payment_id') THEN
CREATE INDEX idx_payment_record_check_logs_payment_id ON payment_record_check_logs(payment_id);
END IF;
END$$;

DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_indexes WHERE indexname = 'idx_payment_record_check_logs_occurred_at') THEN
CREATE INDEX idx_payment_record_check_logs_occurred_at ON payment_record_check_logs(occurred_at);
END IF;
END$$;