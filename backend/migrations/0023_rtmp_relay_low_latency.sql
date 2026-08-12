-- The observed ~2s of extra delay through the relay lines up exactly with
-- the VBV buffer size: -bufsize was set to 2x the target bitrate, which at
-- a CBR bitrate of BITRATE kbps is *2 seconds* worth of data the encoder's
-- rate-control is allowed to hold before it must flush -- that's latency
-- budget, not just headroom, even with -tune zerolatency (which only kills
-- B-frames/lookahead, not the VBV window). Dropping it to 0.5x cuts our
-- own added buffering roughly 4x. This does not touch whatever floor
-- Twitch's own ingest/transcode pipeline imposes on top -- that's outside
-- this relay's control.
UPDATE eggs
SET startup_command = 'set -e
mkdir -p /home/container

PORT="${RTMP_PORT:-1935}"
BITRATE="${BITRATE_KBPS:-6000}"
PRESET="${X264_PRESET:-veryfast}"
LOCAL_URL="rtmp://127.0.0.1:$PORT/$RELAY_SECRET"

FFMPEG_CMD="ffmpeg -fflags nobuffer -i $LOCAL_URL -map 0:v:0 -map 0:a:0 -c:v libx264 -preset $PRESET -tune zerolatency -b:v ${BITRATE}k -maxrate ${BITRATE}k -bufsize $((BITRATE / 2))k -g 60 -keyint_min 60 -pix_fmt yuv420p -c:a aac -b:a 160k -ar 44100 -f flv rtmp://live.twitch.tv/app/$TWITCH_KEY"
if [ -n "$YOUTUBE_KEY" ]; then
  FFMPEG_CMD="$FFMPEG_CMD -map 0:v:0 -map 0:a:0 -c:v libx264 -preset $PRESET -tune zerolatency -b:v ${BITRATE}k -maxrate ${BITRATE}k -bufsize $((BITRATE / 2))k -g 60 -keyint_min 60 -pix_fmt yuv420p -c:a aac -b:a 160k -ar 44100 -f flv rtmp://a.rtmp.youtube.com/live2/$YOUTUBE_KEY"
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
