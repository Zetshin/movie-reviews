package main

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"
)

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Server", "Go")

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
	msg := fmt.Sprintf("Display a detail for movieID %d..", movieID)
	w.Write([]byte(msg))
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
