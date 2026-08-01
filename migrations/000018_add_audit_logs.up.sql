CREATE TABLE audit_logs (
    id UUID PRIMARY KEY,
    user_id UUID REFERENCES "user"(id) ON DELETE SET NULL,
    method VARCHAR(10) NOT NULL,
    path VARCHAR(255) NOT NULL,
    ip VARCHAR(45) NOT NULL,
    user_agent TEXT,
    status_code INTEGER NOT NULL,
    payload TEXT,
    duration_ms BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_audit_logs_user_id ON audit_logs(user_id);
CREATE INDEX idx_audit_logs_created_at ON audit_logs(created_at);
