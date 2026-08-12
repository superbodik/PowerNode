-- Plugin store v1: panel plugins are just eggs (category = 'Plugins')
-- installed as ordinary servers on a node the admin designates as the
-- "plugin host" -- reuses the entire existing egg/server/daemon pipeline,
-- no new orchestration code needed. This flag just marks which registered
-- node that is.
ALTER TABLE nodes ADD COLUMN IF NOT EXISTS is_plugin_host BOOLEAN NOT NULL DEFAULT FALSE;
