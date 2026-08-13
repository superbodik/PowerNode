-- Makes explicit, rather than relying on whatever mediamtx:latest-ffmpeg's
-- bundled default happens to be, how fast a hard OBS crash (process killed,
-- network cable pulled, laptop battery dies -- anything that doesn't send a
-- clean RTMP disconnect) gets noticed. Without a graceful FIN from OBS,
-- mediamtx only detects the dead publisher once no data has arrived for
-- readTimeout -- that's what fires runOnUnavailable and switches to the
-- offline-image fallback (migration 0026). Until that fires, the relay is
-- still "available" from mediamtx's point of view even though nothing is
-- coming in.
--
-- Confirmed via mediamtx's own config reference that the documented default
-- is already 10s (read + write both), which is short enough that Twitch's
-- own ingest tolerates the gap without ending the broadcast on its side --
-- but it's an implicit default on a `:latest` tag that can silently change
-- between mediamtx releases. Pinning it here means a crash is *always*
-- caught within 10s and handed off to the offline image, regardless of
-- what future mediamtx versions ship as their own default.
--
-- The other half of "never drops until I turn it off on the site" needs no
-- new code: the offline-image ffmpeg loops the still image indefinitely
-- (-loop 1 has no end condition) for as long as the relay's container is
-- running, and the existing server Stop action in the panel does a normal
-- docker stop -- SIGTERM then SIGKILL to the whole container after the
-- grace period, which tears down every process in it (mediamtx and any
-- backgrounded offline-image ffmpeg alike), ending the Twitch broadcast on
-- demand.
UPDATE eggs
SET startup_command = 'set -e
mkdir -p /home/container

PORT="${RTMP_PORT:-1935}"
BITRATE="${BITRATE_KBPS:-6000}"
PRESET="${X264_PRESET:-veryfast}"
LOCAL_URL="rtmp://127.0.0.1:$PORT/$RELAY_SECRET"
OFFLINE_PID_FILE=/tmp/offline-relay.pid

X264_OPTS="-x264-params nal-hrd=cbr:force-cfr=1:scenecut=0 -b:v ${BITRATE}k -minrate ${BITRATE}k -maxrate ${BITRATE}k -bufsize $((BITRATE / 2))k -vsync cfr -g 60 -keyint_min 60 -pix_fmt yuv420p"

FFMPEG_CMD="ffmpeg -thread_queue_size 4096 -fflags nobuffer -i $LOCAL_URL -map 0:v:0 -map 0:a:0 -c:v libx264 -preset $PRESET -tune zerolatency $X264_OPTS -c:a aac -b:a 160k -ar 44100 -f flv rtmp://live.twitch.tv/app/$TWITCH_KEY"
if [ -n "$YOUTUBE_KEY" ]; then
  FFMPEG_CMD="$FFMPEG_CMD -map 0:v:0 -map 0:a:0 -c:v libx264 -preset $PRESET -tune zerolatency $X264_OPTS -c:a aac -b:a 160k -ar 44100 -f flv rtmp://a.rtmp.youtube.com/live2/$YOUTUBE_KEY"
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
readTimeout: 10s
writeTimeout: 10s

paths:
  $RELAY_SECRET:
    runOnAvailable: >-
      /on-available.sh
    runOnAvailableRestart: true
    runOnUnavailable: /on-unavailable.sh
CONFEOF

exec /mediamtx /mediamtx.yml'
WHERE name = 'RTMP Relay (offload stream encoding)';
