-- Migration 007: Add item image storage directly on DMASTER
--
-- Stores Base64 image content for inventory item photos.
-- Null means no image assigned.

ALTER TABLE DMASTER ADD ITEM_IMAGE BLOB SUB_TYPE TEXT;
