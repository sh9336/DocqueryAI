CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS document_chunks (
    id SERIAL PRIMARY KEY,
    session_id VARCHAR(64) NOT NULL,
    filename VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    embedding vector(1024),
    page_number INT,
    section VARCHAR(255),
    chunk_index INT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- hnsw does not require pre-existing data at index-creation time (unlike ivfflat which
-- builds k-means centroids and degrades to a single list on an empty table)
CREATE INDEX IF NOT EXISTS idx_document_chunks_embedding
    ON document_chunks USING hnsw (embedding vector_cosine_ops);

CREATE INDEX IF NOT EXISTS idx_document_chunks_filename
    ON document_chunks(filename);

CREATE INDEX IF NOT EXISTS idx_document_chunks_page
    ON document_chunks(page_number);

CREATE INDEX IF NOT EXISTS idx_document_chunks_session
    ON document_chunks(session_id);

CREATE TABLE IF NOT EXISTS chat_history (
    id SERIAL PRIMARY KEY,
    session_id VARCHAR(255),
    question TEXT,
    answer TEXT,
    retrieved_chunks INT[],
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_chat_history_session
    ON chat_history(session_id);
