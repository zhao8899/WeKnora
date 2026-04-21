-- Normalize legacy builtin models into the platform model governance lane.
-- Keep is_builtin for backward compatibility, but make every builtin model
-- explicitly platform-shared so runtime and management logic can converge on
-- "tenant model + platform model".

UPDATE models
SET is_platform = TRUE
WHERE is_builtin = TRUE
  AND is_platform = FALSE;
