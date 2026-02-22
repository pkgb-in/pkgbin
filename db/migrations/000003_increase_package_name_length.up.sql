-- Increase package name length to accommodate longer package names
ALTER TABLE packages ALTER COLUMN name TYPE VARCHAR(255);
