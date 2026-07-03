package main

import (
	"errors"
	"fmt"

	"html/template"
	"net/http"
	"strconv"
	"time"

	"github.com/Zetshin/movie-reviews/internal/models"
)

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Server", "Go")
	movies, err := app.movies.Latest()
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	for _, movie := range movies {
		fmt.Fprintf(w, "%+v\n", movie)
	}

	ts, err := template.ParseFiles("./ui/html/pages/home.tmpl")
	if err != nil {
		app.serverError(w, r, err)
		return

	}

	err = ts.Execute(w, nil)
	if err != nil {
		app.serverError(w, r, err)
	}
}

func (app *application) profile(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Profile :D"))
}
func (app *application) moviesShow(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Movie tries"))
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
	files := []string{
		"./ui/html/base.tmpl",
		"./ui/html/partials/nav.tmpl",
		"./ui/html/pages/detail.tmpl",
	}
	ts, err := template.ParseFiles(files...)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	data := templateData{
		Movie: movie,
	}
	// And then execute them. Notice how we are passing in the snippet
	// data (a models.Snippet struct) as the final parameter?
	err = ts.ExecuteTemplate(w, "base", data)
	if err != nil {
		app.serverError(w, r, err)
	}

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

func (app *application) movieCreate(w http.ResponseWriter, r *http.Request) {
	title := "The Evil Dead"
	description := "Five friends vacation in a remote cabin where they discover an ancient book that unleashes terrifying demonic forces."
	release_date := time.Date(1981, 10, 15, 0, 0, 0, 0, time.UTC)
	poster_image := "the-evil-dead.jpg"
	id, err := app.movies.Insert(title, description, release_date, poster_image)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/movies/%d", id), http.StatusSeeOther)
}
