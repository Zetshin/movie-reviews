package main

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/Zetshin/movie-reviews/internal/models"
	"github.com/Zetshin/movie-reviews/internal/tmdb"
)

// importTMDBMovie is used by the admin import handler. Fetch before starting
// the database transaction, so network calls do not hold database locks.
func (app *application) importTMDBMovie(tmdbID int) (int, error) {
	details, err := app.tmdb.GetMovie(tmdbID)
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

	cast := make([]models.Person, 0, len(details.Credits.Cast))
	for _, actor := range details.Credits.Cast {
		cast = append(cast, models.Person{
			TMDBID:       actor.ID,
			Name:         actor.Name,
			ProfileImage: tmdb.ImageURL(actor.ProfilePath),
		})
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
