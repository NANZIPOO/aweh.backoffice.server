-- Migration: Add soft delete columns to DMASTER
-- Purpose: Enable audit trail for deleted inventory items without losing historical data
-- Required for: Soft delete functionality (maintain referential integrity for GRVs, sales, recipes)

-- Add soft delete flag (CHAR(1) for 'Y'/'N' or NULL for active items)
ALTER TABLE DMASTER ADD DELETED CHAR(1);

-- Add deletion timestamp
ALTER TABLE DMASTER ADD DELETED_DATE TIMESTAMP;

-- Add user who deleted (matches STAFFNAME from STAFFMASTER or user_id from JWT)
ALTER TABLE DMASTER ADD DELETED_BY VARCHAR(100);

COMMIT;
