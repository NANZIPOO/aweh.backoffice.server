-- Migration 006: Normalize legacy boolean-like flags in DMASTER
--
-- Purpose:
--   Some tenant databases contain non-boolean legacy text values
--   (e.g. 'no no') in IS_BASE_VARIANT / IS_SELLABLE, which can break
--   strict bool scanning in Go.
--
-- Behavior:
--   - Canonical truthy values become TRUE: 1, T, TRUE, Y, YES
--   - Everything else becomes FALSE (including NULL/blank/invalid text)
--
-- Notes:
--   - Idempotent: safe to run multiple times.
--   - Keep this data cleanup separate from runtime query normalization.

UPDATE DMASTER
SET IS_BASE_VARIANT =
  CASE
    WHEN UPPER(TRIM(COALESCE(CAST(IS_BASE_VARIANT AS VARCHAR(20)), ''))) IN ('1', 'T', 'TRUE', 'Y', 'YES') THEN TRUE
    ELSE FALSE
  END;

UPDATE DMASTER
SET IS_SELLABLE =
  CASE
    WHEN UPPER(TRIM(COALESCE(CAST(IS_SELLABLE AS VARCHAR(20)), ''))) IN ('1', 'T', 'TRUE', 'Y', 'YES') THEN TRUE
    ELSE FALSE
  END;
