INSERT INTO eggs (category, name, description, docker_image, startup_command, stop_command)
SELECT 'web', 'Static Website (HTML)',
       'Serves plain HTML/CSS/JS with nginx. Upload your site (index.html and friends) to /home/container via the Files tab. Add a port allocation matching the PORT variable below (8080 by default) on the Network tab, and optionally attach a domain to it on the Domains tab.',
       'nginx:alpine',
       'mkdir -p /home/container
if [ ! -f /home/container/index.html ]; then
  printf ''<!doctype html>\n<html>\n<head><title>It works!</title></head>\n<body>\n<h1>It works!</h1>\n<p>Upload your website files to /home/container. This page disappears once index.html exists.</p>\n</body>\n</html>\n'' > /home/container/index.html
fi
printf ''server {\n    listen %s;\n    root /home/container;\n    index index.html index.htm;\n    location / {\n        try_files $uri $uri/ =404;\n    }\n}\n'' "${PORT:-8080}" > /etc/nginx/conf.d/default.conf
exec nginx -g ''daemon off;''',
       'exit'
WHERE NOT EXISTS (SELECT 1 FROM eggs WHERE name = 'Static Website (HTML)');

INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'Port', 'PORT', '8080', TRUE, 'required|integer'
FROM eggs WHERE name = 'Static Website (HTML)'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'PORT');
