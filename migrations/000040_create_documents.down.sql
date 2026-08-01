ALTER TABLE documents DROP CONSTRAINT IF EXISTS fk_documents_current_version;
DROP TABLE IF EXISTS document_versions;
DROP INDEX IF EXISTS idx_documents_deleted_at;
DROP TABLE IF EXISTS documents;
