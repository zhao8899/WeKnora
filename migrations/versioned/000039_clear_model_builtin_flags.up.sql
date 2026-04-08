-- Finalize model governance convergence: platform-shared models replace legacy builtin models.
-- After this migration, model runtime code no longer depends on models.is_builtin.

UPDATE models
SET is_builtin = FALSE
WHERE is_builtin = TRUE
  AND is_platform = TRUE;
