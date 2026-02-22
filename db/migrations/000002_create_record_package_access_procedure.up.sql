
-- Migration to create the record_package_access stored procedure

CREATE OR REPLACE FUNCTION record_package_access(p_name VARCHAR, is_hit BOOLEAN) 
RETURNS VOID AS $$
BEGIN
    -- 1. Try to UPDATE first
    UPDATE packages 
    SET 
        cache_hit = cache_hit + (CASE WHEN is_hit THEN 1 ELSE 0 END),
        cache_miss = cache_miss + (CASE WHEN is_hit THEN 0 ELSE 1 END),
        updated_at = CURRENT_TIMESTAMP
    WHERE name = p_name;

    -- 2. If no rows were affected by the update, then it's a new package
    IF NOT FOUND THEN
        INSERT INTO packages (name, cache_hit, cache_miss)
        VALUES (p_name, 
                CASE WHEN is_hit THEN 1 ELSE 0 END, 
                CASE WHEN is_hit THEN 0 ELSE 1 END);
    END IF;
END;
$$ LANGUAGE plpgsql;