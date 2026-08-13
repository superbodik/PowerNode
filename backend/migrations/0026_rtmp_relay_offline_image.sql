-- Shows a static image on Twitch/YouTube instead of the stream just going
-- dark when the streamer's own side drops (OBS crashes, wifi dies, laptop
-- sleeps) -- as opposed to a server-side relay failure, which is what
-- runOnAvailableRestart already handles by restarting the relay itself.
-- This is the other half: when there's genuinely nothing coming in from
-- OBS, keep *something* live on the platform side rather than nothing.
--
-- Uses mediamtx's runOnUnavailable hook (the counterpart to the existing
-- runOnAvailable) -- fires the moment OBS's RTMP publish drops. Optional:
-- if OFFLINE_IMAGE isn't set, behavior is unchanged from before (stream
-- just stops during an outage, same as always).
INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'Offline image filename (optional, upload via file manager)', 'OFFLINE_IMAGE', '', TRUE, 'nullable'
FROM eggs WHERE name = 'RTMP Relay (offload stream encoding)'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'OFFLINE_IMAGE');

UPDATE eggs
SET startup_command = 'set -e
mkdir -p /home/container

PORT="${RTMP_PORT:-1935}"
BITRATE="${BITRATE_KBPS:-6000}"
PRESET="${X264_PRESET:-veryfast}"
LOCAL_URL="rtmp://127.0.0.1:$PORT/$RELAY_SECRET"
OFFLINE_PID_FILE=/tmp/offline-relay.pid

FFMPEG_CMD="ffmpeg -fflags nobuffer -i $LOCAL_URL -map 0:v:0 -map 0:a:0 -c:v libx264 -preset $PRESET -tune zerolatency -b:v ${BITRATE}k -maxrate ${BITRATE}k -bufsize $((BITRATE / 2))k -g 60 -keyint_min 60 -pix_fmt yuv420p -c:a aac -b:a 160k -ar 44100 -f flv rtmp://live.twitch.tv/app/$TWITCH_KEY"
if [ -n "$YOUTUBE_KEY" ]; then
  FFMPEG_CMD="$FFMPEG_CMD -map 0:v:0 -map 0:a:0 -c:v libx264 -preset $PRESET -tune zerolatency -b:v ${BITRATE}k -maxrate ${BITRATE}k -bufsize $((BITRATE / 2))k -g 60 -keyint_min 60 -pix_fmt yuv420p -c:a aac -b:a 160k -ar 44100 -f flv rtmp://a.rtmp.youtube.com/live2/$YOUTUBE_KEY"
fi

cat > /on-available.sh <<AVAILEOF
#!/bin/sh
if [ -f "$OFFLINE_PID_FILE" ]; then
  kill "\$(cat $OFFLINE_PID_FILE)" 2>/dev/null
  rm -f "$OFFLINE_PID_FILE"
fi
exec $FFMPEG_CMD
AVAILEOF
chmod +x /on-available.sh

OFFLINE_CMD="ffmpeg -loop 1 -re -i /home/container/$OFFLINE_IMAGE -f lavfi -i anullsrc=r=44100:cl=stereo -r 30 -c:v libx264 -preset veryfast -tune stillimage -b:v ${BITRATE}k -maxrate ${BITRATE}k -bufsize $((BITRATE / 2))k -g 60 -keyint_min 60 -pix_fmt yuv420p -c:a aac -b:a 128k -ar 44100 -f flv rtmp://live.twitch.tv/app/$TWITCH_KEY"
if [ -n "$YOUTUBE_KEY" ]; then
  OFFLINE_CMD="$OFFLINE_CMD; ffmpeg -loop 1 -re -i /home/container/$OFFLINE_IMAGE -f lavfi -i anullsrc=r=44100:cl=stereo -r 30 -c:v libx264 -preset veryfast -tune stillimage -b:v ${BITRATE}k -maxrate ${BITRATE}k -bufsize $((BITRATE / 2))k -g 60 -keyint_min 60 -pix_fmt yuv420p -c:a aac -b:a 128k -ar 44100 -f flv rtmp://a.rtmp.youtube.com/live2/$YOUTUBE_KEY"
fi

cat > /on-unavailable.sh <<UNAVAILEOF
#!/bin/sh
# OFFLINE_IMAGE and the file check below are baked in at container start
# (this heredoc runs once, in the outer script) -- if OFFLINE_IMAGE was
# never set, the path collapses to "/home/container/", which -f correctly
# rejects, so there is nothing further to check for the unset case.
if [ ! -f "/home/container/$OFFLINE_IMAGE" ]; then
  exit 0
fi
( $OFFLINE_CMD ) &
echo \$! > $OFFLINE_PID_FILE
UNAVAILEOF
chmod +x /on-unavailable.sh

cat > /mediamtx.yml <<CONFEOF
rtmp: true
rtmpAddress: :$PORT

paths:
  $RELAY_SECRET:
    runOnAvailable: >-
      /on-available.sh
    runOnAvailableRestart: true
    runOnUnavailable: /on-unavailable.sh
CONFEOF

exec /mediamtx /mediamtx.yml'
WHERE name = 'RTMP Relay (offload stream encoding)';
