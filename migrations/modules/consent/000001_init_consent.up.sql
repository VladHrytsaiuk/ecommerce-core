CREATE TABLE legal_documents (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), type VARCHAR(32) NOT NULL, version VARCHAR(64) NOT NULL, content_url VARCHAR(2048) NOT NULL, published_at TIMESTAMPTZ NOT NULL, is_active BOOLEAN NOT NULL DEFAULT false, UNIQUE(type, version));
CREATE UNIQUE INDEX legal_documents_one_active_idx ON legal_documents(type) WHERE is_active;
CREATE TABLE customer_consents (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), customer_id UUID NOT NULL, document_type VARCHAR(32) NOT NULL, document_version VARCHAR(64) NOT NULL, granted_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP, withdrawn_at TIMESTAMPTZ, ip_address VARCHAR(64) NOT NULL);
CREATE UNIQUE INDEX customer_consents_active_idx ON customer_consents(customer_id, document_type, document_version) WHERE withdrawn_at IS NULL;
CREATE INDEX customer_consents_history_idx ON customer_consents(customer_id, granted_at DESC);
CREATE TABLE privacy_requests (id UUID PRIMARY KEY DEFAULT gen_random_uuid(), customer_id UUID NOT NULL, request_type VARCHAR(16) NOT NULL CHECK(request_type IN ('export','erasure')), status VARCHAR(16) NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','in_progress','completed','rejected')), created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP);
CREATE INDEX privacy_requests_customer_idx ON privacy_requests(customer_id, created_at DESC);
