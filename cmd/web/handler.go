package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv" // New import
	"strings"
	"time"

	// New import
	"github.com/Zetshin/movie-reviews/internal/models"
	"github.com/Zetshin/movie-reviews/internal/tmdb"
	"github.com/Zetshin/movie-reviews/internal/validator"
)

// movieSearch returns up to six titles for the Home dropdown.
func (app *application) movieSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if !validator.MaxChars(query, 100) {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	type result struct {
		ID          int    `json:"id"`
		Title       string `json:"title"`
		ReleaseYear string `json:"release_year"`
		PosterImage string `json:"poster_image"`
		AvgRating   string `json:"avg_rating"`
	}
	results := make([]result, 0)
	if query != "" {
		movies, err := app.movies.Search(query, 6)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		for _, movie := range movies {
			year := ""
			if !movie.ReleaseDate.IsZero() {
				year = movie.ReleaseDate.Format("2006")
			}
			results = append(results, result{movie.ID, movie.Title, year, movie.PosterImage, movie.AvgRating})
		}
	}
	payload, err := json.Marshal(results)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(payload)
}

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	// หนังวันฉายล่าสุด: ใช้ทั้ง Hero และ New Movies
	movies, err := app.movies.Latest(6)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// หนังที่ผ่านเกณฑ์ Top Picks
	topMovies, err := app.movies.TopPicks()
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// จับคู่หนังแต่ละเรื่องกับรีวิวของเรื่องนั้น
	var topPicks []HomeTopPick

	for _, movie := range topMovies {
		pick := HomeTopPick{
			Movie: movie,
		}

		review, err := app.reviews.GetRandom(movie.ID)
		if err != nil {
			if !errors.Is(err, models.ErrNoRecord) {
				app.serverError(w, r, err)
				return
			}
			// ไม่มีรีวิวผ่านเงื่อนไข: ReviewPreview เป็น nil
		} else {
			pick.ReviewPreview = &review
		}

		topPicks = append(topPicks, pick)
	}

	// Home ใช้ลำดับ Trending People ที่นำเข้าจาก TMDB; ข้อมูลคนเดิมยังอยู่
	actors, err := app.persons.Trending(6)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// ส่งข้อมูลทั้งหมดให้ Home
	data := app.newTemplateData(r)
	data.Movies = movies
	data.TopPicks = topPicks
	data.HeroMovies, err = app.popularHero(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	data.Actors = actors

	app.render(w, r, http.StatusOK, "home.tmpl", data)
}
func (app *application) profile(w http.ResponseWriter, r *http.Request) {
	userID := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")
	if userID < 1 {
		http.Redirect(w, r, "/user/login", http.StatusSeeOther)
		return
	}
	user, err := app.users.Get(userID)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.sessionManager.Remove(r.Context(), "authenticatedUserID")
			http.Redirect(w, r, "/user/login", http.StatusSeeOther)
		} else {
			app.serverError(w, r, err)
		}
		return
	}
	data := app.newTemplateData(r)
	data.User = user
	data.UserReviews, err = app.reviews.ListByUserID(userID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	app.render(w, r, http.StatusOK, "profile.tmpl", data)
}
func (app *application) moviesShow(w http.ResponseWriter, r *http.Request) {
	page := 1
	if value := r.URL.Query().Get("page"); value != "" {
		var err error
		page, err = strconv.Atoi(value)
		if err != nil || page < 1 {
			http.NotFound(w, r)
			return
		}
	}
	count, err := app.movies.Count()
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	const pageSize = 20
	totalPages := (count + pageSize - 1) / pageSize
	if page > max(1, totalPages) {
		http.NotFound(w, r)
		return
	}
	movies, err := app.movies.ListPage(pageSize, (page-1)*pageSize)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	// Call the newTemplateData() helper to get a templateData struct containing
	// the 'default' data (which for now is just the current year), and add the
	// snippets slice to it.
	data := app.newTemplateData(r)
	data.Movies = movies
	data.Pagination = paginationData{Page: page, TotalPages: totalPages, PreviousPage: page - 1, NextPage: page + 1}
	// Pass the data to the render() helper as normal.
	app.render(w, r, http.StatusOK, "movies.tmpl", data)
}

func (app *application) movieDetail(w http.ResponseWriter, r *http.Request) {
	movieID, err := strconv.Atoi(r.PathValue("movieID"))
	if err != nil || movieID < 1 {
		http.NotFound(w, r)
		return
	}
	movie, err := app.movies.Get(movieID)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			http.NotFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}
	pageSize := 10
	page := 1
	pageString := r.URL.Query().Get("page")
	if pageString != "" {
		page, err = strconv.Atoi(pageString)
		if err != nil || page < 1 {
			http.NotFound(w, r)
			return
		}
	}
	sort := r.URL.Query().Get("sort")
	totalReviews, err := app.reviews.CountByMovieID(movieID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	totalPages := (totalReviews + pageSize - 1) / pageSize
	if page < 1 || (totalPages == 0 && page != 1) || (totalPages > 0 && page > totalPages) {
		http.NotFound(w, r)
		return
	}

	offset := (page - 1) * pageSize
	reviews, err := app.reviews.ListByMovieID(movieID, sort, pageSize, offset)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data.Reviews = reviews
	data.Movie = movie
	data.Cast, err = app.moviePersons.GetCastMembers(movieID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	data.Pagination.Page = page
	data.Pagination.TotalPages = totalPages
	data.Pagination.TotalReviews = totalReviews
	data.Pagination.Sort = sort
	data.Pagination.PreviousPage = page - 1
	data.Pagination.NextPage = page + 1
	app.render(w, r, http.StatusOK, "detail.tmpl", data)

}
func (app *application) movieReview(w http.ResponseWriter, r *http.Request) {
	movieID, err := strconv.Atoi(r.PathValue("movieID"))
	if err != nil || movieID < 1 {
		http.NotFound(w, r)
		return
	}
	movie, err := app.movies.Get(movieID)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			http.NotFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}
	data := app.newTemplateData(r)
	data.Movie = movie
	data.Form = movieReviewForm{}
	app.render(w, r, http.StatusOK, "review.tmpl", data)
}

type movieReviewForm struct {
	Rating              float64 `form:"rating"`
	ReviewText          string  `form:"review_text"`
	IsSpoiler           bool    `form:"is_spoiler"`
	validator.Validator `form:"-"`
}

func (app *application) movieReviewPost(w http.ResponseWriter, r *http.Request) {
	movieID, err := strconv.Atoi(r.PathValue("movieID"))
	if err != nil || movieID < 1 {
		http.NotFound(w, r)
		return
	}
	movie, err := app.movies.Get(movieID)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			http.NotFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}

	var form movieReviewForm
	if err := app.decodePostForm(r, &form); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	form.ReviewText = strings.TrimSpace(form.ReviewText)
	form.CheckField(validator.PermittedValue(form.Rating, 0.5, 1.0, 1.5, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5, 5.0), "rating", "Choose a rating from 0.5 to 5 in half-star increments.")
	form.CheckField(validator.NotBlank(form.ReviewText), "review_text", "Write your review before submitting.")
	form.CheckField(validator.MaxChars(form.ReviewText, 2000), "review_text", "Your review must be 2000 characters or fewer.")
	if !form.Valid() {
		data := app.newTemplateData(r)
		data.Movie = movie
		data.Form = form
		app.render(w, r, http.StatusUnprocessableEntity, "review.tmpl", data)
		return
	}

	// The author comes from the session, never from submitted form data.
	userID := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")
	if userID < 1 {
		http.Redirect(w, r, "/user/login", http.StatusSeeOther)
		return
	}
	_, err = app.reviews.Insert(movieID, userID, form.Rating, form.ReviewText, form.IsSpoiler)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			http.NotFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}
	app.sessionManager.Put(r.Context(), "flash", "Your review has been published.")
	http.Redirect(w, r, fmt.Sprintf("/movies/%d#reviews", movieID), http.StatusSeeOther)
}

func (app *application) movieAdd(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	data.Form = movieAddForm{}
	var err error
	data.Actors, err = app.persons.All()
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	status := app.loadAdminMovies(r, &data)
	app.render(w, r, status, "add.tmpl", data)
}

// The browser imports selected movies one at a time, reporting progress per request.
// This avoids holding a single HTTP response open for an entire batch.
func (app *application) movieImportOnePost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	respond := func(status int, state, message string) {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"status": state, "message": message})
	}
	if err := r.ParseForm(); err != nil {
		respond(http.StatusBadRequest, "failed", "Invalid form.")
		return
	}
	id, err := strconv.Atoi(r.PostForm.Get("tmdb_id"))
	if err != nil || id < 1 {
		respond(http.StatusBadRequest, "failed", "Invalid movie ID.")
		return
	}
	_, err = app.movies.LocalIDForTMDB(id)
	if err != nil && !errors.Is(err, models.ErrNoRecord) {
		app.logger.Error("could not check TMDB movie", "error", err)
		respond(http.StatusInternalServerError, "failed", "Could not check the library.")
		return
	}
	state := "skipped"
	if errors.Is(err, models.ErrNoRecord) {
		state = "imported"
	}
	// Existing movie edits are retained while rankings and credits are refreshed.
	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	defer cancel()
	movieID, err := app.importTMDBMovieContext(ctx, id)
	if err != nil {
		app.logger.Error("TMDB import failed", "tmdb_id", id, "error", err)
		respond(http.StatusBadGateway, "failed", "Movie import failed. Select again to retry.")
		return
	}
	tasks, err := app.moviePersons.ImportTasks(movieID)
	if err != nil {
		app.logger.Error("could not load cast tasks", "movie_id", movieID, "error", err)
		respond(http.StatusInternalServerError, "failed", "Movie is saved, but cast details could not be loaded. Select again to retry.")
		return
	}
	json.NewEncoder(w).Encode(struct {
		Status  string                  `json:"status"`
		MovieID int                     `json:"movie_id"`
		Cast    []models.CastImportTask `json:"cast"`
	}{Status: state, MovieID: movieID, Cast: tasks})
}

// One HTTP request fills one person's missing details; no DB transaction spans the API call.
func (app *application) movieCastImportPost(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	respond := func(code int, state, message string) {
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"status": state, "message": message})
	}
	movieID, err := catalogID(r, "movieID")
	if err != nil {
		respond(http.StatusBadRequest, "failed", "Invalid movie.")
		return
	}
	personID, err := catalogID(r, "personID")
	if err != nil {
		respond(http.StatusBadRequest, "failed", "Invalid actor.")
		return
	}
	task, err := app.moviePersons.ImportTask(movieID, personID)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			respond(http.StatusNotFound, "failed", "This actor is not linked to the movie.")
		} else {
			app.logger.Error("could not read cast task", "error", err)
			respond(http.StatusInternalServerError, "failed", "Could not read this cast member.")
		}
		return
	}
	if !task.NeedsDetails {
		respond(http.StatusOK, "ready", "Existing details retained.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	defer cancel()
	person, err := app.tmdb.GetPersonContext(ctx, task.TMDBID)
	if err != nil {
		app.logger.Error("cast details import failed", "tmdb_id", task.TMDBID, "error", err)
		respond(http.StatusBadGateway, "failed", "Could not fetch actor details. Select the movie again to retry.")
		return
	}
	var birthday *time.Time
	if person.Birthday != nil && *person.Birthday != "" {
		date, err := time.Parse("2006-01-02", *person.Birthday)
		if err != nil {
			respond(http.StatusBadGateway, "failed", "TMDB returned an invalid birth date.")
			return
		}
		birthday = &date
	}
	if err := app.persons.FillTMDBDetails(task.TMDBID, person.Biography, birthday, tmdb.ImageURL(person.ProfilePath)); err != nil {
		app.logger.Error("could not save person details", "error", err)
		respond(http.StatusInternalServerError, "failed", "Could not save actor details.")
		return
	}
	stored, err := app.persons.Get(personID)
	if err != nil {
		respond(http.StatusInternalServerError, "failed", "Could not confirm saved details.")
		return
	}
	if strings.TrimSpace(stored.Bio) == "" {
		respond(http.StatusOK, "no_bio", "TMDB has no Thai or English biography for this actor.")
		return
	}
	respond(http.StatusOK, "updated", "Biography and available details saved.")
}

type movieAddForm struct {
	PersonIDs           []string `form:"person_id"`
	CharacterNames      []string `form:"character_name"`
	Title               string   `form:"title"`
	Description         string   `form:"description"`
	PosterImage         string   `form:"poster_image"`
	BackdropImage       string   `form:"backdrop_image"`
	ReleaseDate         string   `form:"release_date"`
	validator.Validator `form:"-"`
}

type movieCastRow struct {
	PersonID      string
	CharacterName string
}

func (f movieAddForm) CastRows() []movieCastRow {
	rows := make([]movieCastRow, 0)
	for i, id := range f.PersonIDs {
		if i >= 100 {
			break
		}
		row := movieCastRow{PersonID: id}
		if i < len(f.CharacterNames) {
			row.CharacterName = f.CharacterNames[i]
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		rows = append(rows, movieCastRow{})
	}
	return rows
}

func (app *application) movieAddPost(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(12 << 20); err != nil {
			app.clientError(w, http.StatusBadRequest)
			return
		}
		defer r.MultipartForm.RemoveAll()
	}
	if err := r.ParseForm(); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	if r.PostForm.Get("action") == "import_tmdb" {
		app.movieImportTMDBPost(w, r)
		return
	}

	// Declare a new empty instance of the snippetCreateForm struct.
	var form movieAddForm
	err := app.decodePostForm(r, &form)
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	images := readImageUploads(r, &form.Validator, "poster_image", "backdrop_image")
	people, err := app.persons.All()
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	available := make(map[int]bool, len(people))
	for _, person := range people {
		available[person.ID] = true
	}
	credits := make([]models.ManualCastCredit, 0, len(form.PersonIDs))
	seen := make(map[int]bool)
	if len(form.PersonIDs) != len(form.CharacterNames) || len(form.PersonIDs) > 100 {
		form.AddFieldError("cast", "Choose up to 100 actors with matching character fields.")
	} else {
		for i, value := range form.PersonIDs {
			character := strings.TrimSpace(form.CharacterNames[i])
			if value == "" && character == "" {
				continue
			}
			id, parseErr := strconv.Atoi(value)
			if parseErr != nil || !available[id] || seen[id] {
				form.AddFieldError("cast", "Choose an existing actor once per movie.")
				continue
			}
			if len([]rune(character)) > 255 {
				form.AddFieldError("cast", "Character names must be 255 characters or fewer.")
			}
			seen[id] = true
			credits = append(credits, models.ManualCastCredit{PersonID: id, CharacterName: character})
		}
	}
	validated := movieEditForm{Title: form.Title, Description: form.Description, ReleaseDate: form.ReleaseDate, PosterImage: form.PosterImage, BackdropImage: form.BackdropImage}
	if _, ok := images["poster_image"]; ok {
		validated.PosterImage = ""
	}
	if _, ok := images["backdrop_image"]; ok {
		validated.BackdropImage = ""
	}
	releaseDate := validated.validate()
	form.Title = validated.Title
	for field, message := range validated.FieldErrors {
		form.AddFieldError(field, message)
	}

	if !form.Valid() {
		data := app.newTemplateData(r)
		data.Form = form
		data.Actors = people
		app.render(w, r, http.StatusUnprocessableEntity, "add.tmpl", data)
		return
	}

	paths, err := saveImageUploads(images, "movies")
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if path := paths["poster_image"]; path != "" {
		form.PosterImage = path
	}
	if path := paths["backdrop_image"]; path != "" {
		form.BackdropImage = path
	}
	id, err := app.movies.Insert(form.Title, form.Description, releaseDate, strings.TrimSpace(form.PosterImage), strings.TrimSpace(form.BackdropImage), credits...)
	if err != nil {
		removeImageUploads(paths)
		app.serverError(w, r, err)
		return
	}
	app.sessionManager.Put(r.Context(), "flash", "Movie successfully created!")
	http.Redirect(w, r, fmt.Sprintf("/movies/%d", id), http.StatusSeeOther)
}

func (app *application) personDetail(w http.ResponseWriter, r *http.Request) {
	personID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || personID < 1 {
		http.NotFound(w, r)
		return
	}
	person, err := app.persons.Get(personID)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			http.NotFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}
	data := app.newTemplateData(r)
	data.Person = person
	data.PersonMovies, err = app.moviePersons.GetFilmography(personID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	data.ReturnMovie = personOriginMovie(r, data.PersonMovies)
	app.render(w, r, http.StatusOK, "person.tmpl", data)
}

func (app *application) personAdd(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	data.Form = personEditForm{}
	status := app.loadAdminPeople(r, &data)
	app.render(w, r, status, "person-add.tmpl", data)
}

func (app *application) personAddPost(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(12 << 20); err != nil {
			app.clientError(w, http.StatusBadRequest)
			return
		}
		defer r.MultipartForm.RemoveAll()
	}
	if err := r.ParseForm(); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	data := app.newTemplateData(r)
	data.AdminSection = "people"
	data.TMDBCategory = "popular"
	data.TMDBSearch = strings.TrimSpace(r.URL.Query().Get("q"))
	data.Form = personEditForm{}
	if r.PostForm.Get("action") == "import_tmdb" {
		id, err := strconv.Atoi(r.PostForm.Get("tmdb_id"))
		if err != nil || id < 1 {
			app.clientError(w, http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
		defer cancel()
		details, err := app.tmdb.GetPersonContext(ctx, id)
		if err != nil {
			app.logger.Error("TMDB person import failed", "tmdb_id", id, "error", err)
			data.TMDBError = "Could not import this actor. Please try again."
			app.render(w, r, http.StatusBadGateway, "person-add.tmpl", data)
			return
		}
		person := models.Person{TMDBID: details.ID, Name: details.Name, Bio: details.Biography, ProfileImage: tmdb.ImageURL(details.ProfilePath)}
		if details.Birthday != nil && *details.Birthday != "" {
			person.BirthDate, err = time.Parse("2006-01-02", *details.Birthday)
			if err != nil {
				data.TMDBError = "TMDB returned an invalid birth date."
				app.render(w, r, http.StatusBadGateway, "person-add.tmpl", data)
				return
			}
		}
		localID, err := app.persons.ImportTMDB(person)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		app.sessionManager.Put(r.Context(), "flash", "Actor imported. Missing details were filled; existing edits were kept.")
		http.Redirect(w, r, fmt.Sprintf("/actors/%d", localID), http.StatusSeeOther)
		return
	}
	var form personEditForm
	if err := app.decodePostForm(r, &form); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	images := readImageUploads(r, &form.Validator, "profile_image")
	originalImage := form.ProfileImage
	if _, ok := images["profile_image"]; ok {
		form.ProfileImage = ""
	}
	birthday := form.validate()
	if _, ok := images["profile_image"]; ok {
		form.ProfileImage = originalImage
	}
	if !form.Valid() {
		data.Form = form
		app.render(w, r, http.StatusUnprocessableEntity, "person-add.tmpl", data)
		return
	}
	paths, err := saveImageUploads(images, "persons")
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if path := paths["profile_image"]; path != "" {
		form.ProfileImage = path
	}
	id, err := app.persons.Insert(form.Name, form.Bio, birthday, form.ProfileImage)
	if err != nil {
		removeImageUploads(paths)
		app.serverError(w, r, err)
		return
	}
	app.sessionManager.Put(r.Context(), "flash", "Actor added.")
	http.Redirect(w, r, fmt.Sprintf("/actors/%d", id), http.StatusSeeOther)
}

func (app *application) personSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if !validator.MaxChars(query, 100) {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	type result struct {
		ID           int    `json:"id"`
		Name         string `json:"name"`
		ProfileImage string `json:"profile_image"`
	}
	results := make([]result, 0)
	if query != "" {
		persons, err := app.persons.Search(query, 6)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		for _, person := range persons {
			results = append(results, result{person.ID, person.Name, person.ProfileImage})
		}
	}
	payload, err := json.Marshal(results)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(payload)
}

// Create a new userSignupForm struct.
type userSignupForm struct {
	Name                string `form:"name"`
	Email               string `form:"email"`
	Password            string `form:"password"`
	validator.Validator `form:"-"`
}

// Update the handler so it displays the signup page.
func (app *application) userSignup(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	data.Form = userSignupForm{}
	app.render(w, r, http.StatusOK, "signup.tmpl", data)
}
func (app *application) userSignupPost(w http.ResponseWriter, r *http.Request) {
	// Declare a zero-valued instance of our userSignupForm struct.
	var form userSignupForm
	// Parse the form data into the userSignupForm struct.
	err := app.decodePostForm(r, &form)
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	// Validate the form contents using our helper functions.
	form.CheckField(validator.NotBlank(form.Name), "name", "This field cannot be blank")
	form.CheckField(validator.NotBlank(form.Email), "email", "This field cannot be blank")
	form.CheckField(validator.Matches(form.Email, validator.EmailRX), "email", "This field must be a valid email address")
	form.CheckField(validator.NotBlank(form.Password), "password", "This field cannot be blank")
	form.CheckField(validator.MinChars(form.Password, 8), "password", "This field must be at least 8 characters long")
	form.CheckField(validator.MaxBytes(form.Password, 72), "password", "This field must not be more than 72 bytes long")
	// If there are any errors, redisplay the signup form along with a 422
	// status code.
	if !form.Valid() {
		data := app.newTemplateData(r)
		data.Form = form
		app.render(w, r, http.StatusUnprocessableEntity, "signup.tmpl", data)
		return
	}
	// Try to create a new user record in the database. If the email already
	// exists then add an error message to the form and redisplay it.
	err = app.users.Insert(form.Name, form.Email, form.Password)
	if err != nil {
		if errors.Is(err, models.ErrDuplicateEmail) {
			form.AddFieldError("email", "Email address is already in use")
			data := app.newTemplateData(r)
			data.Form = form
			app.render(w, r, http.StatusUnprocessableEntity, "signup.tmpl", data)
		} else {
			app.serverError(w, r, err)
		}
		return
	}
	// Otherwise add a confirmation flash message to the session confirming that
	// their signup worked.
	app.sessionManager.Put(r.Context(), "flash", "Your signup was successful. Please log in.")
	// And redirect the user to the login page.
	http.Redirect(w, r, "/user/login", http.StatusSeeOther)
}

type userLoginForm struct {
	Email               string `form:"email"`
	Password            string `form:"password"`
	validator.Validator `form:"-"`
}

// Update the handler so it displays the login page.
func (app *application) userLogin(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	data.Form = userLoginForm{}
	app.render(w, r, http.StatusOK, "login.tmpl", data)
}

func (app *application) userLoginPost(w http.ResponseWriter, r *http.Request) {
	// Decode the form data into the userLoginForm struct.
	var form userLoginForm
	err := app.decodePostForm(r, &form)
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	// Do some validation checks on the form. We check that both email and
	// password are provided, and that the the format of the email address as
	// a UX-nicety (in case the user makes a typo). We also again make sure that
	// the password provided is no longer than 72 bytes to avoid any bcrypt
	// truncation issues.
	form.CheckField(validator.NotBlank(form.Email), "email", "This field cannot be blank")
	form.CheckField(validator.Matches(form.Email, validator.EmailRX), "email", "This field must be a valid email address")
	form.CheckField(validator.NotBlank(form.Password), "password", "This field cannot be blank")
	form.CheckField(validator.MaxBytes(form.Password, 72), "password", "This field must not be more than 72 bytes long")
	if !form.Valid() {
		data := app.newTemplateData(r)
		data.Form = form
		app.render(w, r, http.StatusUnprocessableEntity, "login.tmpl", data)
		return
	}
	// Check whether the credentials are valid. If they're not, add a generic
	// non-field error message and redisplay the login page.
	id, err := app.users.Authenticate(form.Email, form.Password)
	if err != nil {
		if errors.Is(err, models.ErrInvalidCredentials) {
			form.AddNonFieldError("Email or password is incorrect")
			data := app.newTemplateData(r)
			data.Form = form
			app.render(w, r, http.StatusUnprocessableEntity, "login.tmpl", data)
		} else {
			app.serverError(w, r, err)
		}
		return
	}
	// Use the RenewToken() method on the current session to change the session
	// ID. It's good practice to generate a new session ID when the
	// authentication state or privilege levels change for the user (e.g. login
	// and logout operations).
	err = app.sessionManager.RenewToken(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	// Add the ID of the current user to the session, so that they are now
	// 'logged in'.
	app.sessionManager.Put(r.Context(), "authenticatedUserID", id)
	// Redirect the user to the create snippet page.
	http.Redirect(w, r, "/movies/", http.StatusSeeOther)
}

func (app *application) userLogoutPost(w http.ResponseWriter, r *http.Request) {
	// Use the RenewToken() method on the current session to change the session
	// ID again.
	err := app.sessionManager.RenewToken(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	// Remove the authenticatedUserID from the session data so that the user is
	// 'logged out'.
	app.sessionManager.Remove(r.Context(), "authenticatedUserID")
	// Add a flash message to the session to confirm to the user that they've been
	// logged out.
	app.sessionManager.Put(r.Context(), "flash", "You've been logged out successfully!")
	// Redirect the user to the application home page.
	http.Redirect(w, r, "/movies/", http.StatusSeeOther)
}

func (app *application) profileAvatarPost(w http.ResponseWriter, r *http.Request) {
	userID := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")
	r.Body = http.MaxBytesReader(w, r.Body, 3<<20)
	if err := r.ParseMultipartForm(3 << 20); err != nil {
		app.renderAvatarError(w, r, "Upload a JPG or PNG image no larger than 2 MB.")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, header, err := r.FormFile("avatar")
	if err != nil || header.Size > 2<<20 {
		if file != nil {
			file.Close()
		}
		app.renderAvatarError(w, r, "Choose a JPG or PNG image no larger than 2 MB.")
		return
	}
	defer file.Close()
	config, format, err := image.DecodeConfig(file)
	if err != nil || (format != "jpeg" && format != "png") || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 16000000 {
		app.renderAvatarError(w, r, "Use a valid JPG or PNG image with no more than 16 million pixels.")
		return
	}
	if _, err := file.Seek(0, 0); err != nil {
		app.serverError(w, r, err)
		return
	}
	source, _, err := image.Decode(file)
	if err != nil {
		app.renderAvatarError(w, r, "Could not read this image. Try another JPG or PNG.")
		return
	}
	// Crop centrally and store a small PNG, not the original untrusted upload.
	avatar := squareAvatar(source)
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		app.serverError(w, r, err)
		return
	}
	name := hex.EncodeToString(random[:]) + ".png"
	if err := os.MkdirAll(avatarDirectory, 0755); err != nil {
		app.serverError(w, r, err)
		return
	}
	path := filepath.Join(avatarDirectory, name)
	output, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	encodeErr := png.Encode(output, avatar)
	closeErr := output.Close()
	if encodeErr != nil || closeErr != nil {
		os.Remove(path)
		app.serverError(w, r, errors.Join(encodeErr, closeErr))
		return
	}
	old, err := app.users.SetAvatar(userID, "/static/uploads/avatars/"+name)
	if err != nil {
		os.Remove(path)
		app.serverError(w, r, err)
		return
	}
	// Only remove a generated file in this avatar directory, never an arbitrary path.
	if match := regexp.MustCompile(`^/static/uploads/avatars/([a-f0-9]{32}\.png)$`).FindStringSubmatch(old); len(match) == 2 {
		if err := os.Remove(filepath.Join(avatarDirectory, match[1])); err != nil && !errors.Is(err, os.ErrNotExist) {
			app.logger.Error("could not remove old avatar", "error", err)
		}
	}
	app.sessionManager.Put(r.Context(), "flash", "Your profile photo has been updated.")
	http.Redirect(w, r, "/profile", http.StatusSeeOther)
}

func (app *application) reviewEdit(w http.ResponseWriter, r *http.Request) {
	review, ok := app.ownedReview(w, r)
	if !ok {
		return
	}
	movie, err := app.movies.Get(review.MovieID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	data := app.newTemplateData(r)
	data.Movie, data.EditingReviewID = movie, review.ID
	data.Form = movieReviewForm{Rating: review.Rating, ReviewText: review.Review, IsSpoiler: review.IsSpoiler}
	app.render(w, r, http.StatusOK, "review.tmpl", data)
}

func (app *application) reviewEditPost(w http.ResponseWriter, r *http.Request) {
	review, ok := app.ownedReview(w, r)
	if !ok {
		return
	}
	var form movieReviewForm
	if err := app.decodePostForm(r, &form); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	form.ReviewText = strings.TrimSpace(form.ReviewText)
	form.CheckField(validator.PermittedValue(form.Rating, 0.5, 1.0, 1.5, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5, 5.0), "rating", "Choose a rating from 0.5 to 5 in half-star increments.")
	form.CheckField(validator.NotBlank(form.ReviewText), "review_text", "Write your review before submitting.")
	form.CheckField(validator.MaxChars(form.ReviewText, 2000), "review_text", "Your review must be 2000 characters or fewer.")
	if !form.Valid() {
		movie, err := app.movies.Get(review.MovieID)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		data := app.newTemplateData(r)
		data.Movie, data.EditingReviewID, data.Form = movie, review.ID, form
		app.render(w, r, http.StatusUnprocessableEntity, "review.tmpl", data)
		return
	}
	userID := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")
	if err := app.reviews.UpdateForUser(form.Rating, form.ReviewText, form.IsSpoiler, review.ID, userID); err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			http.NotFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}
	app.sessionManager.Put(r.Context(), "flash", "Your review has been updated.")
	http.Redirect(w, r, fmt.Sprintf("/profile#review-%d", review.ID), http.StatusSeeOther)
}

// peopleShow displays the people catalog using the shared site navigation.
func (app *application) peopleShow(w http.ResponseWriter, r *http.Request) {
	people, err := app.persons.All()
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	data := app.newTemplateData(r)
	data.Actors = people
	app.render(w, r, http.StatusOK, "people.tmpl", data)
}

type movieEditForm struct {
	Title               string `form:"title"`
	Description         string `form:"description"`
	ReleaseDate         string `form:"release_date"`
	PosterImage         string `form:"poster_image"`
	BackdropImage       string `form:"backdrop_image"`
	validator.Validator `form:"-"`
}

type personEditForm struct {
	Name                string `form:"name"`
	Bio                 string `form:"bio"`
	BirthDate           string `form:"birth_date"`
	ProfileImage        string `form:"profile_image"`
	validator.Validator `form:"-"`
}

func optionalDate(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse("2006-01-02", value)
}

// Accept local image paths and web URLs, never executable URL schemes.
func validCatalogImage(value string) bool {
	if value == "" {
		return true
	}
	u, err := url.Parse(value)
	if err != nil || strings.ContainsAny(value, "\r\n\\") {
		return false
	}
	if strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//") {
		return u.Host == ""
	}
	return (u.Scheme == "https" || u.Scheme == "http") && u.Host != "" && u.User == nil
}

func (f *movieEditForm) validate() time.Time {
	f.Title = strings.TrimSpace(f.Title)
	f.PosterImage = strings.TrimSpace(f.PosterImage)
	f.BackdropImage = strings.TrimSpace(f.BackdropImage)
	f.CheckField(validator.NotBlank(f.Title), "title", "Enter a movie title.")
	f.CheckField(validator.MaxChars(f.Title, 255), "title", "Use 255 characters or fewer.")
	f.CheckField(len(f.Description) <= 60000, "description", "Synopsis is too long.")
	date, err := optionalDate(f.ReleaseDate)
	f.CheckField(err == nil, "release_date", "Enter a valid date.")
	f.CheckField(len(f.PosterImage) <= 2048 && validCatalogImage(f.PosterImage), "poster_image", "Use an HTTP/HTTPS URL or a local path starting with /.")
	f.CheckField(len(f.BackdropImage) <= 2048 && validCatalogImage(f.BackdropImage), "backdrop_image", "Use an HTTP/HTTPS URL or a local path starting with /.")
	return date
}

func (f *personEditForm) validate() time.Time {
	f.Name = strings.TrimSpace(f.Name)
	f.ProfileImage = strings.TrimSpace(f.ProfileImage)
	f.CheckField(validator.NotBlank(f.Name), "name", "Enter a name.")
	f.CheckField(validator.MaxChars(f.Name, 255), "name", "Use 255 characters or fewer.")
	f.CheckField(len(f.Bio) <= 60000, "bio", "Biography is too long.")
	date, err := optionalDate(f.BirthDate)
	f.CheckField(err == nil && (date.IsZero() || !date.After(time.Now())), "birth_date", "Enter a valid birth date that is not in the future.")
	f.CheckField(len(f.ProfileImage) <= 2048 && validCatalogImage(f.ProfileImage), "profile_image", "Use an HTTP/HTTPS URL or a local path starting with /.")
	return date
}

func dateInput(date time.Time) string {
	if date.IsZero() {
		return ""
	}
	return date.Format("2006-01-02")
}

func catalogID(r *http.Request, key string) (int, error) {
	id, err := strconv.Atoi(r.PathValue(key))
	if err != nil || id < 1 {
		return 0, models.ErrNoRecord
	}
	return id, nil
}

func (app *application) catalogError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, models.ErrNoRecord) {
		http.NotFound(w, r)
	} else {
		app.serverError(w, r, err)
	}
}

func (app *application) movieEdit(w http.ResponseWriter, r *http.Request) {
	id, err := catalogID(r, "movieID")
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	movie, err := app.movies.Get(id)
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	data := app.newTemplateData(r)
	data.Movie = movie
	data.Form = movieEditForm{Title: movie.Title, Description: movie.Description, ReleaseDate: dateInput(movie.ReleaseDate), PosterImage: movie.PosterImage, BackdropImage: movie.BackdropImage}
	app.render(w, r, http.StatusOK, "movie-edit.tmpl", data)
}

func (app *application) movieEditPost(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseMultipartForm(12 << 20); err != nil {
			app.clientError(w, http.StatusBadRequest)
			return
		}
		defer r.MultipartForm.RemoveAll()
	}
	id, err := catalogID(r, "movieID")
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	movie, err := app.movies.Get(id)
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	var form movieEditForm
	if err := app.decodePostForm(r, &form); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	images := readImageUploads(r, &form.Validator, "poster_image", "backdrop_image")
	originalPoster, originalBackdrop := form.PosterImage, form.BackdropImage
	if _, ok := images["poster_image"]; ok {
		form.PosterImage = ""
	}
	if _, ok := images["backdrop_image"]; ok {
		form.BackdropImage = ""
	}
	date := form.validate()
	if _, ok := images["poster_image"]; ok {
		form.PosterImage = originalPoster
	}
	if _, ok := images["backdrop_image"]; ok {
		form.BackdropImage = originalBackdrop
	}
	if !form.Valid() {
		data := app.newTemplateData(r)
		data.Movie = movie
		data.Form = form
		app.render(w, r, http.StatusUnprocessableEntity, "movie-edit.tmpl", data)
		return
	}
	movie.Title, movie.Description, movie.ReleaseDate = form.Title, form.Description, date
	paths, err := saveImageUploads(images, "movies")
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	if path := paths["poster_image"]; path != "" {
		form.PosterImage = path
	}
	if path := paths["backdrop_image"]; path != "" {
		form.BackdropImage = path
	}
	movie.PosterImage, movie.BackdropImage = form.PosterImage, form.BackdropImage
	if err := app.movies.Update(movie); err != nil {
		removeImageUploads(paths)
		app.catalogError(w, r, err)
		return
	}
	app.sessionManager.Put(r.Context(), "flash", "Movie updated.")
	http.Redirect(w, r, fmt.Sprintf("/movies/%d", id), http.StatusSeeOther)
}

func (app *application) personEdit(w http.ResponseWriter, r *http.Request) {
	id, err := catalogID(r, "id")
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	person, err := app.persons.Get(id)
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	data := app.newTemplateData(r)
	data.Person = person
	data.Form = personEditForm{Name: person.Name, Bio: person.Bio, BirthDate: dateInput(person.BirthDate), ProfileImage: person.ProfileImage}
	app.render(w, r, http.StatusOK, "person-edit.tmpl", data)
}

func (app *application) personEditPost(w http.ResponseWriter, r *http.Request) {
	id, err := catalogID(r, "id")
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	person, err := app.persons.Get(id)
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	var form personEditForm
	if err := app.decodePostForm(r, &form); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	date := form.validate()
	if !form.Valid() {
		data := app.newTemplateData(r)
		data.Person = person
		data.Form = form
		app.render(w, r, http.StatusUnprocessableEntity, "person-edit.tmpl", data)
		return
	}
	person.Name, person.Bio, person.BirthDate, person.ProfileImage = form.Name, form.Bio, date, form.ProfileImage
	if err := app.persons.Update(person); err != nil {
		app.catalogError(w, r, err)
		return
	}
	app.sessionManager.Put(r.Context(), "flash", "Actor updated.")
	http.Redirect(w, r, fmt.Sprintf("/actors/%d", id), http.StatusSeeOther)
}

func (app *application) movieDelete(w http.ResponseWriter, r *http.Request) {
	id, err := catalogID(r, "movieID")
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	movie, err := app.movies.Get(id)
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	data := app.newTemplateData(r)
	data.Movie = movie
	app.render(w, r, http.StatusOK, "movie-delete.tmpl", data)
}

func (app *application) movieDeletePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	id, err := catalogID(r, "movieID")
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	if r.PostForm.Get("confirm") != "delete" {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	if err := app.movies.Delete(id); err != nil {
		app.catalogError(w, r, err)
		return
	}
	app.sessionManager.Put(r.Context(), "flash", "Movie deleted.")
	http.Redirect(w, r, "/movies/", http.StatusSeeOther)
}

func (app *application) personDelete(w http.ResponseWriter, r *http.Request) {
	id, err := catalogID(r, "id")
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	person, err := app.persons.Get(id)
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	data := app.newTemplateData(r)
	data.Person = person
	app.render(w, r, http.StatusOK, "person-delete.tmpl", data)
}

func (app *application) personDeletePost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	id, err := catalogID(r, "id")
	if err != nil {
		app.catalogError(w, r, err)
		return
	}
	if r.PostForm.Get("confirm") != "delete" {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	if err := app.persons.Delete(id); err != nil {
		app.catalogError(w, r, err)
		return
	}
	app.sessionManager.Put(r.Context(), "flash", "Actor deleted.")
	http.Redirect(w, r, "/actors/", http.StatusSeeOther)
}
