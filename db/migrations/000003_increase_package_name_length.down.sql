-- Revert package name length back to VARCHAR(70)
ALTER TABLE packages ALTER COLUMN name TYPE VARCHAR(70);
