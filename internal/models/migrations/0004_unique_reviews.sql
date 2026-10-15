-- Keep the newest review when old data contains several reviews from the
-- same user for the same movie. Likes attached to discarded reviews must be
-- removed first because the original foreign key does not cascade deletes.
DELETE rl
FROM review_likes rl
JOIN reviews older ON older.id = rl.review_id
JOIN reviews newer
  ON newer.movie_id = older.movie_id
 AND newer.user_id = older.user_id
 AND newer.id > older.id;

DELETE older
FROM reviews older
JOIN reviews newer
  ON newer.movie_id = older.movie_id
 AND newer.user_id = older.user_id
 AND newer.id > older.id;

UPDATE movies m
LEFT JOIN (
    SELECT movie_id, COUNT(*) AS review_count, COALESCE(AVG(rating), 0) AS avg_rating
    FROM reviews
    GROUP BY movie_id
) stats ON stats.movie_id = m.id
SET m.review_count = COALESCE(stats.review_count, 0),
    m.avg_rating = COALESCE(stats.avg_rating, 0);

CREATE UNIQUE INDEX IF NOT EXISTS reviews_movie_user_uc ON reviews (movie_id, user_id);
