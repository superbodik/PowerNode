-- "Bitrate drops sometimes" on a live relay is almost always one of three
-- things, and all three are addressable without touching the client:
--
-- 1. No -minrate: with only -b:v/-maxrate set, libx264 is free to spend
--    fewer bits than the target on simple frames (calm scenes, static
--    overlays) -- Twitch's own stream health graph reads that as a real
--    bitrate dip even though nothing is actually wrong. Setting -minrate
--    equal to -maxrate/-b:v forces true CBR: libx264 pads the bitstream
--    up to the target instead of ever dipping below it.
-- 2. No -x264-params nal-hrd=cbr: without HRD signaling, the encoder's
--    internal rate control can still drift under -tune zerolatency; adding
--    it (paired with -minrate=-maxrate above, which nal-hrd=cbr requires
--    to do anything) makes the output bitstream compliant CBR end to end.
--    scenecut=0 rides along in the same -x264-params -- a scene cut would
--    otherwise insert an unplanned keyframe outside the fixed -g interval,
--    which is a genuine sudden bitrate spike, not just a health-graph
--    artifact.
-- 3. No -thread_queue_size on the input: under brief scheduling jitter
--    reading from mediamtx's local RTMP loopback, ffmpeg's default input
--    queue is small enough to log "Thread message queue blocking" and
--    drop frames -- which shows up exactly as intermittent stutter/bitrate
--    loss. A larger queue absorbs that jitter instead of dropping frames
--    for it. -vsync cfr keeps frame delivery to the encoder metronomic on
--    top of that, so minor input timing jitter doesn't turn into duplicated
--    or dropped frames downstream.
--
-- None of this touches the -bufsize halving from the earlier latency fix
-- (migration 0023) -- minrate/maxrate padding doesn't need extra buffer
-- headroom to stay smooth, so the low-latency VBV window is untouched.
--
-- What this can't fix: if the instability is actually the streamer's own
-- upload link to the relay dropping packets, no server-side encoder flag
-- can invent bandwidth that isn't there -- that would show up as OBS's own
-- dropped-frames counter climbing, not just Twitch's health graph, and
-- needs a fresh OBS log to diagnose separately.
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

paths:
  $RELAY_SECRET:
    runOnAvailable: >-
      /on-available.sh
    runOnAvailableRestart: true
    runOnUnavailable: /on-unavailable.sh
CONFEOF

exec /mediamtx /mediamtx.yml'
WHERE name = 'RTMP Relay (offload stream encoding)';
