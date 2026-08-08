-- Ports a server owner added themselves from the server's Network tab.
-- Admin-provisioned pool allocations stay false so releasing them returns
-- them to the node's pool instead of deleting the row.
ALTER TABLE allocations
    ADD COLUMN IF NOT EXISTS is_user_created BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX IF NOT EXISTS idx_allocations_server_id ON allocations (server_id);
