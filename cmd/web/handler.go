package main

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings" // New import
	"time"
	"unicode/utf8" // New import

	"github.com/Zetshin/movie-reviews/internal/models"
)

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	panic("oops! something went wrong")

}

func (app *application) profile(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Profile :D"))
}
func (app *application) moviesShow(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Server", "Go")
	movies, err := app.movies.Latest()
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	// Call the newTemplateData() helper to get a templateData struct containing
	// the 'default' data (which for now is just the current year), and add the
	// snippets slice to it.
	data := app.newTemplateData(r)
	data.Movies = movies
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
	data := app.newTemplateData(r)
	data.Movie = movie
	app.render(w, r, http.StatusOK, "detail.tmpl", data)

}
func (app *application) movieReview(w http.ResponseWriter, r *http.Request) {
	movieID, err := strconv.Atoi(r.PathValue("movieID"))
	if err != nil || movieID < 1 {
		http.NotFound(w, r)
		return
	}
	msg := fmt.Sprintf("Display a specific review for ID %d..", movieID)
	w.Write([]byte(msg))
}

func (app *application) movieReviewPost(w http.ResponseWriter, r *http.Request) {
	movieID, err := strconv.Atoi(r.PathValue("movieID"))
	if err != nil || movieID < 1 {
		http.NotFound(w, r)
		return
	}
	msg := fmt.Sprintf("Created review for ID %d..", movieID)
	w.Write([]byte(msg))
}

func (app *application) movieAdd(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)

	data.Form = movieAddForm{}
	app.render(w, r, http.StatusOK, "add.tmpl", data)
}

type movieAddForm struct {
	Title       string
	Description string
	ReleaseDate time.Time
	PosterImage string
	FieldErrors map[string]string
}

func (app *application) movieAddPost(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	releaseDate, err := time.Parse(
		"2006-01-02",
		r.PostForm.Get("release_date"),
	)
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	form := movieAddForm{
		Title:       r.PostForm.Get("title"),
		Description: r.PostForm.Get("description"),
		PosterImage: r.PostForm.Get("poster_image"),
		ReleaseDate: releaseDate,
		FieldErrors: map[string]string{},
	}
	if strings.TrimSpace(form.Title) == "" {
		form.FieldErrors["title"] = "This field cannot be blank"
	} else if utf8.RuneCountInString(form.Title) > 100 {
		form.FieldErrors["title"] = "This field cannot be more than 100 characters long"
	}
	if len(form.FieldErrors) > 0 {
		data := app.newTemplateData(r)
		data.Form = form
		app.render(w, r, http.StatusUnprocessableEntity, "add.tmpl", data)
		return
	}
	id, err := app.movies.Insert(form.Title, form.Description, form.ReleaseDate, form.PosterImage)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/movies/%d", id), http.StatusSeeOther)
}

func (app *application) personDetail(w http.ResponseWriter, r *http.Request) {
	personID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || personID < 1 {
		http.NotFound(w, r)
		return
	}
	msg := fmt.Sprintf("Display a specific person with ID %d...", personID)
	w.Write([]byte(msg))
}
