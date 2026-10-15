-- Databases created by hand (from the old README) or by earlier versions of
-- the app have the tables but not the schema the queries need, because 0001
-- uses CREATE TABLE IF NOT EXISTS. Bring such databases up to date.
-- ADD COLUMN IF NOT EXISTS and CREATE INDEX IF NOT EXISTS are MariaDB
-- syntax; the migrator emulates them so the same file also works on MySQL 8
-- (see execMigrationStatement).
-- MODIFY COLUMN statements are idempotent: re-running them is a no-op.

-- users ----------------------------------------------------------------
ALTER TABLE users ADD COLUMN IF NOT EXISTS hashed_password CHAR(60) NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS created DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP;
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_admin BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS avatar_image VARCHAR(255) NULL;
-- The old README had a "password" column that the code never writes;
-- make it optional so inserts that only set hashed_password succeed.
ALTER TABLE users ADD COLUMN IF NOT EXISTS password VARCHAR(255) NULL;
ALTER TABLE users MODIFY COLUMN password VARCHAR(255) NULL;

-- movies ---------------------------------------------------------------
ALTER TABLE movies ADD COLUMN IF NOT EXISTS tmdb_id INT NULL;
ALTER TABLE movies ADD COLUMN IF NOT EXISTS backdrop_image TEXT NULL;
-- Imported and hand-entered movies may lack a date or images.
ALTER TABLE movies MODIFY COLUMN release_date DATE NULL;
ALTER TABLE movies MODIFY COLUMN poster_image TEXT NULL;

-- persons --------------------------------------------------------------
ALTER TABLE persons ADD COLUMN IF NOT EXISTS tmdb_id INT NULL;
ALTER TABLE persons ADD COLUMN IF NOT EXISTS trending_rank INT NULL;
-- TMDB cast members are inserted with only a name and photo.
ALTER TABLE persons MODIFY COLUMN bio TEXT NULL;
ALTER TABLE persons MODIFY COLUMN birth_date DATE NULL;
ALTER TABLE persons MODIFY COLUMN profile_image TEXT NULL;

-- movie_persons --------------------------------------------------------
ALTER TABLE movie_persons ADD COLUMN IF NOT EXISTS cast_order INT NULL;
ALTER TABLE movie_persons ADD COLUMN IF NOT EXISTS character_name TEXT NULL;

-- TMDB imports rely on ON DUPLICATE KEY UPDATE, which needs these unique
-- keys; without them every re-import would insert a duplicate row.
CREATE UNIQUE INDEX IF NOT EXISTS movies_uc_tmdb_id ON movies (tmdb_id);
CREATE UNIQUE INDEX IF NOT EXISTS persons_uc_tmdb_id ON persons (tmdb_id);

-- reviews --------------------------------------------------------------
ALTER TABLE reviews ADD COLUMN IF NOT EXISTS is_spoiler BOOLEAN NOT NULL DEFAULT FALSE;
-- Ratings are half stars (0.5-5.0); an INT column silently rounds them.
ALTER TABLE reviews MODIFY COLUMN rating DECIMAL(2,1) NOT NULL;
