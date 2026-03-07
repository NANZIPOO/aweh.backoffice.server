-- 001_create_generators.sql
-- Creates Firebird generators needed for product grouping system
-- Must run FIRST, before 002_, 003_, 004_ migrations

-- Generator for GroupID (INVENTORY_GROUPS.GROUP_ID)
CREATE GENERATOR GroupIdGen;
SET GENERATOR GroupIdGen TO 1000;

-- Generator for MovementID (STOCK_MOVEMENTS.MOVEMENT_ID)
CREATE GENERATOR MovementIdGen;
SET GENERATOR MovementIdGen TO 1000;

-- Verify generators were created
-- SELECT GEN_ID(GroupIdGen, 1) FROM RDB$DATABASE;
-- SELECT GEN_ID(MovementIdGen, 1) FROM RDB$DATABASE;
