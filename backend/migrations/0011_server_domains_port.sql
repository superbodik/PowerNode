-- Which of the server's ports a domain proxies to. Backfilled from each
-- domain's server's primary allocation so existing domains keep working
-- exactly as before (they always used the primary port).
ALTER TABLE server_domains ADD COLUMN port INT;

UPDATE server_domains sd
SET port = (
    SELECT a.port FROM allocations a
    WHERE a.server_id = sd.server_id
    ORDER BY a.id LIMIT 1
)
WHERE sd.port IS NULL;
