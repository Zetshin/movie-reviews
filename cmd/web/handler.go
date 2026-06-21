package main

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
)

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Server", "Go")

	ts, err := template.ParseFiles("./ui/html/pages/home.tmpl")
	if err != nil {
		app.logger.Error(err.Error(), r.Method, "uri", r.URL.RequestURI())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return

	}

	err = ts.Execute(w, nil)
	if err != nil {
		app.logger.Error(err.Error(), r.Method, "uri", r.URL.RequestURI())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func profile(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Profile :D"))
}
func moviesShow(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("Movie tries"))
}
func movieDetail(w http.ResponseWriter, r *http.Request) {
	movieID, err := strconv.Atoi(r.PathValue("movieID"))
	if err != nil || movieID < 1 {
		http.NotFound(w, r)
		return
	}
	msg := fmt.Sprintf("Display a detail for movieID %d..", movieID)
	w.Write([]byte(msg))
}
func movieReview(w http.ResponseWriter, r *http.Request) {
	movieID, err := strconv.Atoi(r.PathValue("movieID"))
	if err != nil || movieID < 1 {
		http.NotFound(w, r)
		return
	}
	msg := fmt.Sprintf("Display a specific review for ID %d..", movieID)
	w.Write([]byte(msg))
}

func movieReviewPost(w http.ResponseWriter, r *http.Request) {
	movieID, err := strconv.Atoi(r.PathValue("movieID"))
	if err != nil || movieID < 1 {
		http.NotFound(w, r)
		return
	}
	msg := fmt.Sprintf("Created review for ID %d..", movieID)
	w.Write([]byte(msg))
}
