-- Every egg that needs to be reachable (RTMP relay, the Telegram proxy,
-- Streamer Stats Site, the static HTML egg) has the exact same two-step
-- friction today: create the server, THEN separately go add a port
-- allocation on the Network tab that happens to match whatever number you
-- typed into the egg's *_PORT variable -- get the two out of sync and the
-- container publishes a port nothing is listening on, or listens on a port
-- nothing publishes. is_port marks which single variable, if any, an egg
-- wants synced to whatever port the server actually gets allocated, so the
-- create-server form can drive both from one choice instead of two.
--
-- auto_generate marks a variable (RELAY_SECRET today) that the create-server
-- form should fill with a random value up front instead of leaving blank
-- for the user to invent one -- the same convenience TWITCH_KEY already got
-- via a Twitch-account lookup, just for a value nothing external can supply.
-- Deliberately not applied to PROXY_SECRET: that one already auto-generates
-- and persists itself server-side (0031) specifically so leaving it blank
-- keeps working the same way across restarts -- having the form fill it
-- client-side would silently defeat that and make the "leave blank to
-- auto-generate and keep" label a lie.
ALTER TABLE egg_variables ADD COLUMN is_port BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE egg_variables ADD COLUMN auto_generate BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE egg_variables SET is_port = TRUE
WHERE egg_id IN (SELECT id FROM eggs WHERE name = 'RTMP Relay (offload stream encoding)')
AND env_variable = 'RTMP_PORT';

UPDATE egg_variables SET is_port = TRUE
WHERE egg_id IN (SELECT id FROM eggs WHERE name = 'Telegram MTProto Proxy')
AND env_variable = 'PROXY_PORT';

UPDATE egg_variables SET is_port = TRUE
WHERE egg_id IN (SELECT id FROM eggs WHERE name = 'Streamer Stats Site')
AND env_variable = 'PORT';

UPDATE egg_variables SET is_port = TRUE
WHERE egg_id IN (SELECT id FROM eggs WHERE name = 'Static Website (HTML)')
AND env_variable = 'PORT';

UPDATE egg_variables SET auto_generate = TRUE
WHERE egg_id IN (SELECT id FROM eggs WHERE name = 'RTMP Relay (offload stream encoding)')
AND env_variable = 'RELAY_SECRET';
