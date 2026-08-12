-- 0014 fixed the crash-loop and the Server/Stream Key convention, and both
-- are confirmed working end to end (OBS -> relay -> Twitch). Remaining
-- complaint: noticeable end-to-end delay. Two of the biggest, well-known
-- latency sources in an ffmpeg relay like this are entirely on the encode
-- side and were left at ffmpeg defaults:
--
--   1. libx264 defaults to using B-frames and rate-control lookahead, which
--      both require buffering several frames before any of them can be
--      emitted -- pure reordering delay, independent of network or CPU.
--   2. ffmpeg itself does some input probing/buffering by default before it
--      starts processing, adding fixed startup latency.
--
-- -tune zerolatency disables B-frames/lookahead in libx264 (standard,
-- well-documented tradeoff: marginally worse compression for the same
-- quality, in exchange for no reordering delay -- the right tradeoff for a
-- live relay). -fflags nobuffer/-flags low_delay minimize ffmpeg's own
-- demux/decode buffering, and -probesize 32 -analyzeduration 0 skip the
-- input format analysis pass (safe here since the input is always the same
-- known H264/AAC RTMP stream from OBS, not an unknown file).
--
-- This does not touch Twitch's own ingest/CDN latency, which has its own
-- floor independent of this relay (viewers should also check "Low Latency
-- Streaming" under the streamer's own Twitch dashboard settings).
UPDATE eggs
SET startup_command = 'set -e
mkdir -p /home/container

PORT="${RTMP_PORT:-1935}"
BITRATE="${BITRATE_KBPS:-6000}"
PRESET="${X264_PRESET:-veryfast}"
LOCAL_URL="rtmp://127.0.0.1:$PORT/$RELAY_SECRET"

FFMPEG_CMD="ffmpeg -fflags nobuffer -flags low_delay -probesize 32 -analyzeduration 0 -i $LOCAL_URL -c:v libx264 -preset $PRESET -tune zerolatency -b:v ${BITRATE}k -maxrate ${BITRATE}k -bufsize $((BITRATE * 2))k -g 60 -keyint_min 60 -pix_fmt yuv420p -c:a aac -b:a 160k -ar 44100 -f flv rtmp://live.twitch.tv/app/$TWITCH_KEY"
if [ -n "$YOUTUBE_KEY" ]; then
  FFMPEG_CMD="$FFMPEG_CMD -c:v libx264 -preset $PRESET -tune zerolatency -b:v ${BITRATE}k -maxrate ${BITRATE}k -bufsize $((BITRATE * 2))k -g 60 -keyint_min 60 -pix_fmt yuv420p -c:a aac -b:a 160k -ar 44100 -f flv rtmp://a.rtmp.youtube.com/live2/$YOUTUBE_KEY"
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
