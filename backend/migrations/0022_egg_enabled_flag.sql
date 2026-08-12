-- Lets an egg be pulled out of the default catalog and gated behind the
-- plugin store instead -- "install the plugin" for a feature-toggle plugin
-- (as opposed to a container plugin, see 0021) means flipping this flag on
-- its egg rather than spinning up a container.
ALTER TABLE eggs ADD COLUMN IF NOT EXISTS enabled BOOLEAN NOT NULL DEFAULT TRUE;

-- Cut streaming out of the default catalog per explicit request -- it's now
-- opt-in via the Plugins page instead of available to every user out of
-- the box.
UPDATE eggs SET enabled = FALSE WHERE category = 'streaming';
