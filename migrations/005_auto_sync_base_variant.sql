-- Migration 005: Auto-sync INVENTORY_GROUPS.BASE_ITEMPARTNO with DMASTER.IS_BASE_VARIANT
-- When IS_BASE_VARIANT is set to TRUE on any DMASTER row, automatically:
--   1. Update INVENTORY_GROUPS.BASE_ITEMPARTNO to point to that item
--   2. Set IS_BASE_VARIANT = FALSE on the previous base item

CREATE OR ALTER TRIGGER TRG_DMASTER_BASE_VARIANT_SYNC
  FOR DMASTER
  ACTIVE AFTER UPDATE
AS
  DECLARE VARIABLE old_base_item BIGINT;
BEGIN
  -- Only fire if IS_BASE_VARIANT changed from false to true AND item belongs to a group
  IF (NEW.IS_BASE_VARIANT = TRUE 
      AND (OLD.IS_BASE_VARIANT IS NULL OR OLD.IS_BASE_VARIANT = FALSE)
      AND NEW.GROUP_ID IS NOT NULL) THEN
  BEGIN
    -- Get the current base item for this group
    SELECT BASE_ITEMPARTNO 
    FROM INVENTORY_GROUPS 
    WHERE GROUP_ID = NEW.GROUP_ID
    INTO :old_base_item;

    -- If there was a previous base item (and it's not the same item), unset it
    IF (:old_base_item IS NOT NULL AND :old_base_item <> NEW.ITEMPARTNO) THEN
    BEGIN
      UPDATE DMASTER 
      SET IS_BASE_VARIANT = FALSE
      WHERE ITEMPARTNO = :old_base_item;
    END

    -- Update the group's base item reference
    UPDATE INVENTORY_GROUPS 
    SET BASE_ITEMPARTNO = NEW.ITEMPARTNO,
        BASE_UOM = NEW.UOM
    WHERE GROUP_ID = NEW.GROUP_ID;
  END
END
