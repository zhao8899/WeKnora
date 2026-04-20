-- Migration: 000031_message_feedback
-- Description: Add feedback column to messages for thumbs-up/down quality signal
DO $$ BEGIN RAISE NOTICE '[Migration 000031] Adding feedback column to messages'; END $$;

ALTER TABLE messages ADD COLUMN IF NOT EXISTS feedback VARCHAR(10) NOT NULL DEFAULT '';
COMMENT ON COLUMN messages.feedback IS 'User quality feedback: empty string (no feedback), ''like'', or ''dislike''';

DO $$ BEGIN RAISE NOTICE '[Migration 000031] feedback column added to messages successfully'; END $$;
