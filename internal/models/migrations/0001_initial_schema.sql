-- Initial schema matching the queries in internal/models.
-- Every statement is idempotent so it is safe on databases that were
-- created by hand before migrations existed.

CREATE TABLE IF NOT EXISTS users (
    id              INT AUTO_INCREMENT PRIMARY KEY,
    username        VARCHAR(255) NOT NULL,
    email           VARCHAR(255) NOT NULL,
    hashed_password CHAR(60) NOT NULL,
    created         DATETIME NOT NULL,
    is_admin        BOOLEAN NOT NULL DEFAULT FALSE,
    avatar_image    VARCHAR(255) NULL,
    CONSTRAINT users_uc_email UNIQUE (email)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS movies (
    id             INT AUTO_INCREMENT PRIMARY KEY,
    tmdb_id        INT NULL,
    title          VARCHAR(255) NOT NULL,
    description    TEXT NOT NULL,
    release_date   DATE NULL,
    poster_image   TEXT NULL,
    backdrop_image TEXT NULL,
    review_count   INT NOT NULL DEFAULT 0,
    avg_rating     DECIMAL(3,2) NOT NULL DEFAULT 0.00,
    CONSTRAINT movies_uc_tmdb_id UNIQUE (tmdb_id),
    INDEX movies_release_date_idx (release_date),
    INDEX movies_title_idx (title)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS persons (
    id            INT AUTO_INCREMENT PRIMARY KEY,
    tmdb_id       INT NULL,
    name          VARCHAR(255) NOT NULL,
    bio           TEXT NULL,
    birth_date    DATE NULL,
    profile_image TEXT NULL,
    trending_rank INT NULL,
    CONSTRAINT persons_uc_tmdb_id UNIQUE (tmdb_id),
    INDEX persons_name_idx (name),
    INDEX persons_trending_rank_idx (trending_rank)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS movie_persons (
    movie_id       INT NOT NULL,
    person_id      INT NOT NULL,
    role           VARCHAR(100) NOT NULL,
    cast_order     INT NULL,
    character_name TEXT NULL,
    PRIMARY KEY (movie_id, person_id),
    INDEX movie_persons_person_idx (person_id),
    CONSTRAINT movie_persons_fk_movie FOREIGN KEY (movie_id) REFERENCES movies (id),
    CONSTRAINT movie_persons_fk_person FOREIGN KEY (person_id) REFERENCES persons (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS reviews (
    id          INT AUTO_INCREMENT PRIMARY KEY,
    movie_id    INT NOT NULL,
    user_id     INT NOT NULL,
    rating      DECIMAL(2,1) NOT NULL,
    review_text TEXT NOT NULL,
    is_spoiler  BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    -- ListByMovieID and GetRandom scan this into time.Time, so it must not be NULL.
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX reviews_movie_created_idx (movie_id, created_at),
    INDEX reviews_user_idx (user_id),
    CONSTRAINT reviews_fk_movie FOREIGN KEY (movie_id) REFERENCES movies (id),
    CONSTRAINT reviews_fk_user FOREIGN KEY (user_id) REFERENCES users (id),
    CONSTRAINT reviews_rating_range CHECK (rating >= 0.5 AND rating <= 5.0)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS review_likes (
    review_id INT NOT NULL,
    user_id   INT NOT NULL,
    PRIMARY KEY (review_id, user_id),
    INDEX review_likes_user_idx (user_id),
    CONSTRAINT review_likes_fk_review FOREIGN KEY (review_id) REFERENCES reviews (id),
    CONSTRAINT review_likes_fk_user FOREIGN KEY (user_id) REFERENCES users (id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Required by github.com/alexedwards/scs/mysqlstore.
CREATE TABLE IF NOT EXISTS sessions (
    token  CHAR(43) PRIMARY KEY,
    data   BLOB NOT NULL,
    expiry TIMESTAMP(6) NOT NULL,
    INDEX sessions_expiry_idx (expiry)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
