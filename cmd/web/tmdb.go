package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Zetshin/movie-reviews/internal/models"
	"github.com/Zetshin/movie-reviews/internal/tmdb"
	"github.com/Zetshin/movie-reviews/internal/validator"
)

// loadAdminMovies loads one browsable page; browsing never writes catalog records.
func (app *application) loadAdminMovies(r *http.Request, data *templateData) int {
	data.TMDBCategory = r.URL.Query().Get("category")
	if data.TMDBCategory == "" {
		data.TMDBCategory = "popular"
	}
	data.TMDBSearch = strings.TrimSpace(r.URL.Query().Get("q"))
	data.TMDBPage = 1
	if value := r.URL.Query().Get("page"); value != "" {
		page, err := strconv.Atoi(value)
		if err != nil || page < 1 || page > 500 {
			data.TMDBError = "Choose a page between 1 and 500."
			return http.StatusBadRequest
		}
		data.TMDBPage = page
	}
	if !validator.PermittedValue(data.TMDBCategory, "popular", "now_playing", "search") || !validator.MaxChars(data.TMDBSearch, 100) {
		data.TMDBError = "Choose a valid category and use at most 100 characters for search."
		return http.StatusBadRequest
	}
	// A typed title always means search, including submissions without JavaScript.
	if data.TMDBSearch != "" {
		data.TMDBCategory = "search"
	}
	if data.TMDBCategory == "search" && data.TMDBSearch == "" {
		return http.StatusOK
	}
	var list tmdb.MovieList
	var err error
	switch data.TMDBCategory {
	case "search":
		list, err = app.tmdb.SearchMovies(data.TMDBSearch, data.TMDBPage)
	case "now_playing":
		list, err = app.tmdb.NowPlayingMovies(data.TMDBPage)
	default:
		list, err = app.tmdb.DiscoverMovies(data.TMDBPage)
	}
	data.TMDBSearchDone = true
	if err != nil {
		app.logger.Error("TMDB browse failed", "error", err)
		data.TMDBError = "Could not load TMDB movies. Please try again."
		return http.StatusBadGateway
	}
	data.TMDBMovies = list.Results
	data.TMDBTotalPages = min(list.TotalPages, 500)
	ids := make([]int, 0, len(list.Results))
	for _, movie := range list.Results {
		ids = append(ids, movie.ID)
	}
	data.TMDBImported, err = app.movies.ImportedTMDBIDs(ids)
	if err != nil {
		data.TMDBError = "Could not check your movie library. Please try again."
		return http.StatusInternalServerError
	}
	pageURL := func(page int) string {
		params := url.Values{"category": {data.TMDBCategory}, "page": {strconv.Itoa(page)}}
		if data.TMDBSearch != "" {
			params.Set("q", data.TMDBSearch)
		}
		return "/movies/add?" + params.Encode()
	}
	if data.TMDBPage > 1 {
		data.TMDBPreviousURL = pageURL(data.TMDBPage - 1)
	}
	if data.TMDBPage < data.TMDBTotalPages {
		data.TMDBNextURL = pageURL(data.TMDBPage + 1)
	}
	return http.StatusOK
}

func (app *application) loadAdminPeople(r *http.Request, data *templateData) int {
	data.AdminSection = "people"
	data.TMDBCategory = r.URL.Query().Get("category")
	if data.TMDBCategory == "" {
		data.TMDBCategory = "popular"
	}
	data.TMDBSearch = strings.TrimSpace(r.URL.Query().Get("q"))
	data.TMDBPage = 1
	if value := r.URL.Query().Get("page"); value != "" {
		page, err := strconv.Atoi(value)
		if err != nil || page < 1 || page > 500 {
			data.TMDBError = "Choose a page between 1 and 500."
			return http.StatusBadRequest
		}
		data.TMDBPage = page
	}
	if !validator.PermittedValue(data.TMDBCategory, "popular", "search") || !validator.MaxChars(data.TMDBSearch, 100) {
		data.TMDBError = "Choose a valid category and use at most 100 characters for search."
		return http.StatusBadRequest
	}
	if data.TMDBSearch != "" {
		data.TMDBCategory = "search"
	}
	if data.TMDBCategory == "search" && data.TMDBSearch == "" {
		return http.StatusOK
	}
	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	defer cancel()
	var list tmdb.PeopleList
	var err error
	if data.TMDBCategory == "search" {
		list, err = app.tmdb.SearchPeople(ctx, data.TMDBSearch, data.TMDBPage)
	} else {
		list, err = app.tmdb.PopularPeople(ctx, data.TMDBPage)
	}
	data.TMDBSearchDone = true
	if err != nil {
		app.logger.Error("TMDB people browse failed", "error", err)
		data.TMDBError = "Could not load TMDB actors. Please try again."
		return http.StatusBadGateway
	}
	data.TMDBPeople = list.Results
	data.TMDBTotalPages = min(list.TotalPages, 500)
	ids := make([]int, 0, len(list.Results))
	for _, person := range list.Results {
		ids = append(ids, person.ID)
	}
	data.TMDBImported, err = app.persons.ImportedTMDBIDs(ids)
	if err != nil {
		data.TMDBError = "Could not check your actor library."
		return http.StatusInternalServerError
	}
	pageURL := func(page int) string {
		params := url.Values{"category": {data.TMDBCategory}, "page": {strconv.Itoa(page)}}
		if data.TMDBSearch != "" {
			params.Set("q", data.TMDBSearch)
		}
		return "/actors/add?" + params.Encode()
	}
	if data.TMDBPage > 1 {
		data.TMDBPreviousURL = pageURL(data.TMDBPage - 1)
	}
	if data.TMDBPage < data.TMDBTotalPages {
		data.TMDBNextURL = pageURL(data.TMDBPage + 1)
	}
	return http.StatusOK
}

// importTMDBMovie is used by the admin import handler. Fetch before starting
// the database transaction, so network calls do not hold database locks.
func (app *application) importTMDBMovie(tmdbID int) (int, error) {
	return app.importTMDBMovieContext(context.Background(), tmdbID)
}

// Explicit backfill for existing movies; preserve editorial content and reviews.
func (app *application) syncTMDBMetadata() error {
	ids, err := app.movies.ImportedIDs()
	if err != nil {
		return err
	}
	failed := 0
	for index, id := range ids {
		ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
		_, err := app.importTMDBMovieContext(ctx, id)
		cancel()
		if err != nil {
			failed++
			app.logger.Error("movie metadata refresh failed", "tmdb_id", id, "error", err)
		}
		app.logger.Info("metadata sync progress", "processed", index+1, "total", len(ids))
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d movies failed; successful changes were saved", failed, len(ids))
	}
	app.logger.Info("metadata sync complete", "movies", len(ids))
	return nil
}

func (app *application) importTMDBMovieContext(ctx context.Context, tmdbID int) (int, error) {
	details, err := app.tmdb.GetMovieContext(ctx, tmdbID)
	if err != nil {
		return 0, err
	}
	if details.ID != tmdbID {
		return 0, fmt.Errorf("TMDB returned an unexpected movie ID")
	}

	movie := models.Movie{
		TMDBID:        details.ID,
		Title:         details.Title,
		Description:   details.Overview,
		PosterImage:   tmdb.ImageURL(details.PosterPath),
		BackdropImage: tmdb.BackdropURL(details.BackdropPath),
	}
	if details.ReleaseDate != "" {
		movie.ReleaseDate, err = time.Parse("2006-01-02", details.ReleaseDate)
		if err != nil {
			return 0, fmt.Errorf("invalid TMDB release date: %w", err)
		}
	}

	cast := make([]models.ImportedCastMember, 0, len(details.Credits.Cast))
	for _, actor := range details.Credits.Cast {
		cast = append(cast, models.ImportedCastMember{Order: actor.Order, CharacterName: actor.Character, Person: models.Person{
			TMDBID:       actor.ID,
			Name:         actor.Name,
			ProfileImage: tmdb.ImageURL(actor.ProfilePath),
		}})
	}
	return app.movies.ImportTMDB(movie, cast)
}

// Called only through POST /movies/add, behind the existing admin and CSRF middleware.
func (app *application) movieImportTMDBPost(w http.ResponseWriter, r *http.Request) {
	tmdbID, err := strconv.Atoi(r.PostForm.Get("tmdb_id"))
	if err != nil || tmdbID < 1 {
		data := app.newTemplateData(r)
		data.Form = movieAddForm{}
		data.TMDBError = "Please select a valid TMDB movie."
		app.render(w, r, http.StatusUnprocessableEntity, "add.tmpl", data)
		return
	}
	movieID, err := app.importTMDBMovie(tmdbID)
	if err != nil {
		app.logger.Error("TMDB import failed", "tmdb_id", tmdbID, "error", err)
		data := app.newTemplateData(r)
		data.Form = movieAddForm{}
		data.TMDBError = "Could not import the movie. Please try again."
		app.render(w, r, http.StatusInternalServerError, "add.tmpl", data)
		return
	}
	app.sessionManager.Put(r.Context(), "flash", "Movie imported from TMDB. Existing records were reused if already present.")
	http.Redirect(w, r, fmt.Sprintf("/movies/%d", movieID), http.StatusSeeOther)
}

// importNowPlayingMovies populates the initial catalog on an explicit CLI run.
// Each movie uses the same transaction and duplicate handling as the admin page.
func (app *application) importNowPlayingMovies(limit int) error {
	if limit < 1 || limit > 20 {
		return fmt.Errorf("starter import limit must be between 1 and 20")
	}
	list, err := app.tmdb.NowPlayingMovies(1)
	if err != nil {
		return err
	}
	if len(list.Results) == 0 {
		return fmt.Errorf("TMDB returned no now-playing movies for Thailand")
	}
	// Select the most recent release dates from the first page. This is not
	// an exhaustive list of all TMDB movies or all cinema releases.
	sort.SliceStable(list.Results, func(i, j int) bool {
		return list.Results[i].ReleaseDate > list.Results[j].ReleaseDate
	})
	if len(list.Results) > limit {
		list.Results = list.Results[:limit]
	}
	// Import older selections first. Latest() independently sorts by release date.
	for i := len(list.Results) - 1; i >= 0; i-- {
		movie := list.Results[i]
		movieID, err := app.importTMDBMovie(movie.ID)
		if err != nil {
			return fmt.Errorf("import TMDB movie %d: %w", movie.ID, err)
		}
		app.logger.Info("imported TMDB movie", "tmdb_id", movie.ID, "movie_id", movieID, "title", movie.Title)
	}
	app.logger.Info("TMDB starter import complete", "movies", len(list.Results))
	return nil
}

// importPersonDetails is explicit batch work, not a request made on every page view.
func (app *application) importPersonDetails() error {
	ids, err := app.persons.TMDBIDs()
	if err != nil {
		return err
	}
	failed := 0
	for i, id := range ids {
		person, err := app.tmdb.GetPerson(id)
		if err == nil {
			var birthday *time.Time
			if person.Birthday != nil && *person.Birthday != "" {
				var date time.Time
				date, err = time.Parse("2006-01-02", *person.Birthday)
				if err == nil {
					birthday = &date
				}
			}
			if err == nil {
				err = app.persons.FillTMDBDetails(id, person.Biography, birthday, tmdb.ImageURL(person.ProfilePath))
			}
		}
		if err != nil {
			failed++
			app.logger.Error("could not import person details", "tmdb_id", id, "error", err)
		}
		if (i+1)%20 == 0 || i+1 == len(ids) {
			app.logger.Info("TMDB person import progress", "processed", i+1, "total", len(ids))
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d of %d person imports failed; successful records were saved", failed, len(ids))
	}
	app.logger.Info("TMDB person details import complete", "people", len(ids))
	return nil
}

func (app *application) importTrendingPeople(limit int) error {
	if limit < 1 || limit > 20 {
		return fmt.Errorf("trending people limit must be between 1 and 20")
	}
	list, err := app.tmdb.TrendingPeople()
	if err != nil {
		return err
	}
	people := make([]models.Person, 0, limit)
	seen := make(map[int]bool)
	for _, candidate := range list.Results {
		if candidate.ID < 1 || seen[candidate.ID] || tmdb.ImageURL(candidate.ProfilePath) == "" {
			continue
		}
		details, err := app.tmdb.GetPerson(candidate.ID)
		if err != nil {
			return fmt.Errorf("get trending person %d: %w", candidate.ID, err)
		}
		person := models.Person{
			TMDBID: candidate.ID, Name: details.Name, Bio: details.Biography,
			ProfileImage: tmdb.ImageURL(candidate.ProfilePath),
		}
		if details.Birthday != nil && *details.Birthday != "" {
			person.BirthDate, err = time.Parse("2006-01-02", *details.Birthday)
			if err != nil {
				return fmt.Errorf("invalid birthday for trending person %d: %w", candidate.ID, err)
			}
		}
		people = append(people, person)
		seen[candidate.ID] = true
		if len(people) == limit {
			break
		}
	}
	if err := app.persons.SetTrending(people); err != nil {
		return err
	}
	for i, p := range people {
		app.logger.Info("imported trending person", "rank", i+1, "tmdb_id", p.TMDBID, "name", p.Name)
	}
	return nil
}
