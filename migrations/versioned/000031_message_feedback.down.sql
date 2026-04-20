-- Rollback: 000031_message_feedback
ALTER TABLE messages DROP COLUMN IF EXISTS feedback;
