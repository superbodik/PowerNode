-- 0013 shipped nginx:alpine + `apk add nginx-mod-rtmp`, which crash-loops:
-- nginx-mod-rtmp only exists in Alpine's edge branch, not the stable branch
-- nginx:alpine is actually built on, so the apk install fails, `set -e`
-- kills the script, and Docker's restart policy loops the container forever
-- (surfaces in the panel as "could not attach to container ... received
-- 409" on the Console tab, since the container never reaches a stable
-- running state long enough to attach to).
--
-- Switched to bluenviron/mediamtx:latest-ffmpeg — a single static binary
-- with RTMP support and ffmpeg baked into the image at build time, so
-- there's no runtime package install (and therefore no repo-branch
-- mismatch) at all.
UPDATE eggs
SET docker_image = 'bluenviron/mediamtx:latest-ffmpeg',
    startup_command = 'set -e
mkdir -p /home/container

PORT="${RTMP_PORT:-1935}"
BITRATE="${BITRATE_KBPS:-6000}"
PRESET="${X264_PRESET:-veryfast}"
LOCAL_URL="rtmp://127.0.0.1:$PORT/$RELAY_SECRET"

FFMPEG_CMD="ffmpeg -i $LOCAL_URL -c:v libx264 -preset $PRESET -b:v ${BITRATE}k -maxrate ${BITRATE}k -bufsize $((BITRATE * 2))k -g 60 -keyint_min 60 -pix_fmt yuv420p -c:a aac -b:a 160k -ar 44100 -f flv rtmp://live.twitch.tv/app/$TWITCH_KEY"
if [ -n "$YOUTUBE_KEY" ]; then
  FFMPEG_CMD="$FFMPEG_CMD -c:v libx264 -preset $PRESET -b:v ${BITRATE}k -maxrate ${BITRATE}k -bufsize $((BITRATE * 2))k -g 60 -keyint_min 60 -pix_fmt yuv420p -c:a aac -b:a 160k -ar 44100 -f flv rtmp://a.rtmp.youtube.com/live2/$YOUTUBE_KEY"
fi

cat > /mediamtx.yml <<CONFEOF
rtmp: true
rtmpAddress: :$PORT

paths:
  $RELAY_SECRET:
    runOnAvailable: >-
      $FFMPEG_CMD
CONFEOF

exec /mediamtx /mediamtx.yml'
WHERE name = 'RTMP Relay (offload stream encoding)';
