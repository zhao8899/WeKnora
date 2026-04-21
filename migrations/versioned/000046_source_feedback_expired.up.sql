-- Allow 'expired' as a valid feedback value in addition to 'up' and 'down'.

ALTER TABLE source_feedback
    DROP CONSTRAINT IF EXISTS source_feedback_feedback_check;

ALTER TABLE source_feedback
    ADD CONSTRAINT source_feedback_feedback_check
    CHECK (feedback IN ('up', 'down', 'expired'));
