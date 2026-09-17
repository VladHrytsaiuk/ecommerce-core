-- Notifications owns rendered delivery work and provider audit records. It
-- keeps logical references to Core events/orders without cross-module FKs.
CREATE TABLE notification_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    template_key VARCHAR(128) NOT NULL,
    channel VARCHAR(32) NOT NULL CHECK (channel IN ('email')),
    locale VARCHAR(10) NOT NULL,
    version INTEGER NOT NULL CHECK (version > 0),
    subject_template TEXT NOT NULL,
    html_template TEXT NOT NULL,
    text_template TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (template_key, channel, locale, version)
);
CREATE UNIQUE INDEX notification_templates_active_unique
    ON notification_templates (template_key, channel, locale)
    WHERE is_active;

CREATE TABLE notification_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL,
    order_id UUID NOT NULL,
    dedupe_key VARCHAR(255) NOT NULL UNIQUE,
    channel VARCHAR(32) NOT NULL CHECK (channel IN ('email')),
    recipient VARCHAR(320) NOT NULL,
    locale VARCHAR(10) NOT NULL,
    template_key VARCHAR(128) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'sent', 'failed')),
    provider VARCHAR(64) NOT NULL,
    provider_message_id VARCHAR(255),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error TEXT,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (recipient <> ''),
    CHECK (locale <> '')
);
CREATE INDEX notification_jobs_order_idx ON notification_jobs (order_id, created_at DESC);

CREATE TABLE notification_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id UUID NOT NULL REFERENCES notification_jobs(id) ON DELETE CASCADE,
    attempt_number INTEGER NOT NULL CHECK (attempt_number > 0),
    provider VARCHAR(64) NOT NULL,
    provider_message_id VARCHAR(255),
    status VARCHAR(32) NOT NULL CHECK (status IN ('success', 'failed')),
    error_code VARCHAR(128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (job_id, attempt_number)
);
