-- Rollback migration 000033: Remove dedicated FAQ columns from chunks table

DROP INDEX IF EXISTS idx_chunks_faq_question_lookup;
DROP INDEX IF EXISTS idx_chunks_has_generated_questions;
DROP INDEX IF EXISTS idx_chunks_standard_question;

ALTER TABLE chunks DROP COLUMN IF EXISTS has_generated_questions;
ALTER TABLE chunks DROP COLUMN IF EXISTS standard_question;
