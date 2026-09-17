CREATE TABLE session (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id UUID NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
  refresh_token VARCHAR(255) NOT NULL UNIQUE,
  user_agent VARCHAR(255),
  client_ip VARCHAR(45),
  is_blocked BOOLEAN NOT NULL DEFAULT FALSE,
  expires_at TIMESTAMPTZ NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Додаємо індекс для швидкого пошуку сесії за токеном (бо бекенд буде часто це робити)
CREATE INDEX idx_session_refresh_token ON session(refresh_token);