package tmdb

// Movie holds movie data returned by TMDB, separate from our database model.
type Movie struct {
	ID           int     `json:"id"`
	Title        string  `json:"title"`
	Overview     string  `json:"overview"`
	ReleaseDate  string  `json:"release_date"`
	PosterPath   *string `json:"poster_path"`
	BackdropPath *string `json:"backdrop_path"`
	GenreIDs     []int   `json:"genre_ids"`
}

// MovieList is one page of the Discover response.
type MovieList struct {
	Page         int     `json:"page"`
	Results      []Movie `json:"results"`
	TotalPages   int     `json:"total_pages"`
	TotalResults int     `json:"total_results"`
}

// ImageURL returns a display URL, or an empty string when TMDB has no image.
func ImageURL(path *string) string {
	if path == nil || *path == "" {
		return ""
	}
	return "https://image.tmdb.org/t/p/w500" + *path
}

// BackdropURL uses a wider image for the Home hero.
func BackdropURL(path *string) string {
	if path == nil || *path == "" {
		return ""
	}
	return "https://image.tmdb.org/t/p/w1280" + *path
}
