-- 0015 added -flags low_delay and -probesize 32 -analyzeduration 0 to
-- cut latency. Confirmed via a live console log (the console-visibility fix
-- from an earlier migration is what made this diagnosable at all) that
-- -flags low_delay broke audio: the log showed "Negative cts, previous
-- timestamps might be wrong" and "Invalid timestamps" right as decoding
-- started, and the session ended with audio:275KiB muxed over a ~226s
-- stream at a nominal 160kbit/s (should be roughly 4.4MB) -- the vast
-- majority of audio packets were silently dropped downstream of that
-- timestamp corruption. -flags low_delay (AV_CODEC_FLAG_LOW_DELAY) tells
-- the decoder to skip its frame-reordering buffer, which only works
-- correctly for B-frame-free input; OBS's local encode commonly does use
-- B-frames, so the flag was corrupting decode order for the exact case this
-- relay exists to support.
--
-- Dropped -flags low_delay entirely (keeping the much safer -fflags
-- nobuffer, which only affects demuxer buffering, not decode reordering)
-- and also dropped -probesize 32 -analyzeduration 0 -- stream detection
-- happened to succeed with them in the captured log, but there's no good
-- reason to keep an aggressive, fragile setting that contributed nothing
-- proven once the actual corruption source is gone. -tune zerolatency is
-- untouched and unrelated to this bug -- it's a pure libx264 encoder
-- setting with no effect on demuxing/decoding.
--
-- Also added explicit -map 0:v:0 -map 0:a:0 on both output legs: if a
-- track genuinely goes undetected in the future, ffmpeg now exits loudly
-- instead of silently muxing video-only.
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
CONFEOF

exec /mediamtx /mediamtx.yml'
WHERE name = 'RTMP Relay (offload stream encoding)';
