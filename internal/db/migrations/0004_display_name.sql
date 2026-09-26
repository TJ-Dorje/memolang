-- Optional name shown in the nav instead of the email. Empty means "not set";
-- models.User.Name falls back to the email. A plain ADD COLUMN is enough here:
-- NOT NULL is allowed because the column has a constant default.
ALTER TABLE users ADD COLUMN display_name TEXT NOT NULL DEFAULT '';
