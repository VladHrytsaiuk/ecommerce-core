CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id UUID NOT NULL UNIQUE REFERENCES domain_events(id) ON DELETE RESTRICT,
    actor_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    action VARCHAR(150) NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    resource_id UUID NOT NULL,
    old_payload JSONB,
    new_payload JSONB,
    ip_address INET,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    occurred_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (action <> ''),
    CHECK (resource_type <> '')
);

CREATE INDEX audit_logs_actor_occurred_idx ON audit_logs (actor_user_id, occurred_at DESC);
CREATE INDEX audit_logs_resource_occurred_idx ON audit_logs (resource_type, resource_id, occurred_at DESC);
CREATE INDEX audit_logs_action_occurred_idx ON audit_logs (action, occurred_at DESC);
