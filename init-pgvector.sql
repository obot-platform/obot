-- Initialize pgvector extension for Obot
-- This script is run automatically when the PostgreSQL container starts

-- Create the vector extension
CREATE EXTENSION IF NOT EXISTS vector;

-- Verify the extension was created
SELECT * FROM pg_extension WHERE extname = 'vector';