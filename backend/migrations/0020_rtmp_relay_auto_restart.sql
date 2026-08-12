-- Root cause of "stream just stops and OBS has to be fully restarted to
-- recover": mediamtx runs the relay ffmpeg via runOnAvailable, which fires
-- once when the path transitions from unavailable -> available (OBS
-- connects). If ffmpeg then dies for any reason while OBS stays connected
-- (a transient network drop talking to Twitch, ffmpeg has no built-in
-- reconnect for its RTMP output) -- mediamtx does NOT relaunch it, since
-- the path never toggled unavailable in between to re-trigger
-- runOnAvailable. The RTMP ingest from OBS keeps flowing into mediamtx
-- fine the whole time (which is why OBS itself reports a healthy
-- connection), but nothing relays it to Twitch anymore. Only a full
-- stop+restart in OBS forces a real unavailable->available transition and
-- gets a fresh ffmpeg process.
--
-- runOnAvailableRestart: true tells mediamtx to relaunch the command
-- immediately if it exits while the path is still available, closing this
-- gap without needing the client to do anything.
UPDATE eggs
SET startup_command = 'set -e
mkdir -p /home/container

PORT="${RTMP_PORT:-1935}"
BITRATE="${BITRATE_KBPS:-6000}"
PRESET="${X264_PRESET:-veryfast}"
LOCAL_URL="rtmp://127.0.0.1:$PORT/$RELAY_SECRET"

FFMPEG_CMD="ffmpeg -fflags nobuffer -i $LOCAL_URL -map 0:v:0 -map 0:a:0 -c:v libx264 -preset $PRESET -tune zerolatency -b:v ${BITRATE}k -maxrate ${BITRATE}k -bufsize $((BITRATE * 2))k -g 60 -keyint_min 60 -pix_fmt yuv420p -c:a aac -b:a 160k -ar 44100 -f flv rtmp://live.twitch.tv/app/$TWITCH_KEY"
if [ -n "$YOUTUBE_KEY" ]; then
  FFMPEG_CMD="$FFMPEG_CMD -map 0:v:0 -map 0:a:0 -c:v libx264 -preset $PRESET -tune zerolatency -b:v ${BITRATE}k -maxrate ${BITRATE}k -bufsize $((BITRATE * 2))k -g 60 -keyint_min 60 -pix_fmt yuv420p -c:a aac -b:a 160k -ar 44100 -f flv rtmp://a.rtmp.youtube.com/live2/$YOUTUBE_KEY"
fi

cat > /mediamtx.yml <<CONFEOF
rtmp: true
rtmpAddress: :$PORT

paths:
  $RELAY_SECRET:
    runOnAvailable: >-
      $FFMPEG_CMD
    runOnAvailableRestart: true
CONFEOF

exec /mediamtx /mediamtx.yml'
WHERE name = 'RTMP Relay (offload stream encoding)';
