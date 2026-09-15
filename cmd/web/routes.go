package main

import (
	"net/http"

	"github.com/justinas/alice"
)

func (app *application) routes() http.Handler {

	mux := http.NewServeMux()
	fileServer := http.FileServer(http.Dir("./ui/static/"))
	mux.Handle("GET /static/", http.StripPrefix("/static", fileServer))

	mux.HandleFunc("GET /{$}", app.home)
	mux.HandleFunc("GET /profile", app.profile)
	mux.HandleFunc("GET /movies/", app.moviesShow)           //แสดงหนังคร่าวๆ
	mux.HandleFunc("GET /movies/{movieID}", app.movieDetail) //แสดลงรายละเอียดหนัง มีรีวิวคร่าวๆอยู่ด้วบ
	mux.HandleFunc("GET /movies/add", app.movieAdd)
	mux.HandleFunc("POST /movies/add", app.movieAddPost)                 //ให้เพิ่มหนัง
	mux.HandleFunc("GET /movies/{movieID}/review", app.movieReview)      //แสดงรีวิวทั้งหมด
	mux.HandleFunc("POST /movies/{movieID}/review", app.movieReviewPost) //ส่งรีวิว
	mux.HandleFunc("GET /persons/{$}", app.personDetail)
	standard := alice.New(app.recoverPanic, app.logRequest, commonHeaders)
	// Return the 'standard' middleware chain followed by the servemux.
	return standard.Then(mux)

}
