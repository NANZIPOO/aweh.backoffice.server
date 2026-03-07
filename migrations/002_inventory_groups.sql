-- Migration: Create INVENTORY_GROUPS table for product variants grouping
-- Purpose: Link multiple DMASTER records (variants) to a single product group
-- Author: System
-- Date: 2026-03-06

CREATE TABLE INVENTORY_GROUPS (
    GROUP_ID           BIGINT PRIMARY KEY,
    BASE_ITEMPARTNO    BIGINT NOT NULL,
    GROUP_NAME         VARCHAR(100) NOT NULL,
    BASE_UOM           VARCHAR(50) NOT NULL,
    CREATED_AT         TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IDX_GROUPS_BASE ON INVENTORY_GROUPS(BASE_ITEMPARTNO);
