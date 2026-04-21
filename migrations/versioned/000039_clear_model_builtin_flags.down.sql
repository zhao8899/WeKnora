-- Restore the legacy builtin marker for platform models that were previously normalized.

UPDATE models
SET is_builtin = TRUE
WHERE is_platform = TRUE
  AND is_builtin = FALSE;
