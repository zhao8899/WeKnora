-- Roll back the explicit platform flag added to legacy builtin models.
-- Only rows still marked is_builtin are affected so native platform models are preserved.

UPDATE models
SET is_platform = FALSE
WHERE is_builtin = TRUE;
