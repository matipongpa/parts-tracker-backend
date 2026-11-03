-- Closes the race in identity.Register's pre-check. Fails if duplicate usernames already exist.
CREATE UNIQUE INDEX IF NOT EXISTS users_username_key ON users (username);
