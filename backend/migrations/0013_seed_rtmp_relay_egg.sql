INSERT INTO eggs (category, name, description, docker_image, startup_command, stop_command)
SELECT 'streaming', 'RTMP Relay (offload stream encoding)',
       'Point OBS at this server instead of Twitch directly and it does the heavy encoding for you, freeing up your gaming PC. In OBS: Server = rtmp://<this server''s address>:<RTMP_PORT>/<RELAY_SECRET>, Stream Key = live. Add a port allocation matching RTMP_PORT (1935 by default) on the Network tab so OBS can reach it. Keep RELAY_SECRET private — anyone who has it can publish through your relay onto your Twitch/YouTube channel under your key.',
       'nginx:alpine',
       'set -e
mkdir -p /home/container
apk add --no-cache nginx-mod-rtmp ffmpeg >/dev/null

PORT="${RTMP_PORT:-1935}"
BITRATE="${BITRATE_KBPS:-6000}"
PRESET="${X264_PRESET:-veryfast}"
LOCAL_URL="rtmp://127.0.0.1:$PORT/$RELAY_SECRET/live"

CONF=/etc/nginx/nginx.conf
cat > "$CONF" <<CONFEOF
load_module modules/ngx_rtmp_module.so;
worker_processes auto;
rtmp_auto_push on;
events {}

rtmp {
    server {
        listen $PORT;
        chunk_size 4096;

        application $RELAY_SECRET {
            live on;
            record off;

            exec_push ffmpeg -i $LOCAL_URL -c:v libx264 -preset $PRESET -b:v ${BITRATE}k -maxrate ${BITRATE}k -bufsize $((BITRATE * 2))k -g 60 -keyint_min 60 -pix_fmt yuv420p -c:a aac -b:a 160k -ar 44100 -f flv "rtmp://live.twitch.tv/app/$TWITCH_KEY";
CONFEOF

if [ -n "$YOUTUBE_KEY" ]; then
  cat >> "$CONF" <<CONFEOF2
            exec_push ffmpeg -i $LOCAL_URL -c:v libx264 -preset $PRESET -b:v ${BITRATE}k -maxrate ${BITRATE}k -bufsize $((BITRATE * 2))k -g 60 -keyint_min 60 -pix_fmt yuv420p -c:a aac -b:a 160k -ar 44100 -f flv "rtmp://a.rtmp.youtube.com/live2/$YOUTUBE_KEY";
CONFEOF2
fi

cat >> "$CONF" <<CONFEOF3
        }
    }
}
CONFEOF3

exec nginx -g ''daemon off;''',
       'exit'
WHERE NOT EXISTS (SELECT 1 FROM eggs WHERE name = 'RTMP Relay (offload stream encoding)');

INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'Relay secret', 'RELAY_SECRET', '', TRUE, 'required|min:12'
FROM eggs WHERE name = 'RTMP Relay (offload stream encoding)'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'RELAY_SECRET');

INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'Twitch stream key', 'TWITCH_KEY', '', TRUE, 'required'
FROM eggs WHERE name = 'RTMP Relay (offload stream encoding)'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'TWITCH_KEY');

INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'YouTube stream key (optional, simulcasts too)', 'YOUTUBE_KEY', '', TRUE, 'nullable'
FROM eggs WHERE name = 'RTMP Relay (offload stream encoding)'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'YOUTUBE_KEY');

INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'Output bitrate (kbps)', 'BITRATE_KBPS', '6000', TRUE, 'required|integer'
FROM eggs WHERE name = 'RTMP Relay (offload stream encoding)'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'BITRATE_KBPS');

INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'x264 preset (ultrafast..veryslow)', 'X264_PRESET', 'veryfast', TRUE, 'required|in:ultrafast,superfast,veryfast,faster,fast,medium,slow,slower,veryslow'
FROM eggs WHERE name = 'RTMP Relay (offload stream encoding)'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'X264_PRESET');

INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'RTMP ingest port', 'RTMP_PORT', '1935', TRUE, 'required|integer'
FROM eggs WHERE name = 'RTMP Relay (offload stream encoding)'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'RTMP_PORT');
