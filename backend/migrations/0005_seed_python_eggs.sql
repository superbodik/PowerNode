INSERT INTO eggs (category, name, description, docker_image, startup_command, stop_command)
SELECT 'python', 'Python: Website', 'Generic Python web app (Flask/Django/FastAPI/etc). Upload your code to /home/container, list dependencies in requirements.txt, and set an entrypoint filename via the START_FILE variable (default app.py).', 'python:3.12-slim', 'pip install --no-cache-dir -r requirements.txt 2>/dev/null; python3 ${START_FILE:-app.py}', 'stop'
WHERE NOT EXISTS (SELECT 1 FROM eggs WHERE name = 'Python: Website');

INSERT INTO eggs (category, name, description, docker_image, startup_command, stop_command)
SELECT 'python', 'Python: Telegram Bot', 'Generic Python Telegram bot (python-telegram-bot, aiogram, etc). Upload your code to /home/container, list dependencies in requirements.txt, and set an entrypoint filename via the START_FILE variable (default bot.py).', 'python:3.12-slim', 'pip install --no-cache-dir -r requirements.txt 2>/dev/null; python3 ${START_FILE:-bot.py}', 'stop'
WHERE NOT EXISTS (SELECT 1 FROM eggs WHERE name = 'Python: Telegram Bot');

INSERT INTO eggs (category, name, description, docker_image, startup_command, stop_command)
SELECT 'python', 'Python: Discord Bot', 'Generic Python Discord bot (discord.py, py-cord, etc). Upload your code to /home/container, list dependencies in requirements.txt, and set an entrypoint filename via the START_FILE variable (default bot.py).', 'python:3.12-slim', 'pip install --no-cache-dir -r requirements.txt 2>/dev/null; python3 ${START_FILE:-bot.py}', 'stop'
WHERE NOT EXISTS (SELECT 1 FROM eggs WHERE name = 'Python: Discord Bot');

INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'Start file', 'START_FILE', 'app.py', TRUE, 'required|string'
FROM eggs WHERE name = 'Python: Website'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'START_FILE');

INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'Start file', 'START_FILE', 'bot.py', TRUE, 'required|string'
FROM eggs WHERE name = 'Python: Telegram Bot'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'START_FILE');

INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'Start file', 'START_FILE', 'bot.py', TRUE, 'required|string'
FROM eggs WHERE name = 'Python: Discord Bot'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'START_FILE');

INSERT INTO egg_variables (egg_id, name, env_variable, default_value, is_editable, rules)
SELECT id, 'Accept Mojang EULA', 'EULA', 'FALSE', TRUE, 'required|in:TRUE,FALSE'
FROM eggs WHERE name = 'Minecraft: Vanilla'
AND NOT EXISTS (SELECT 1 FROM egg_variables WHERE egg_id = eggs.id AND env_variable = 'EULA');
