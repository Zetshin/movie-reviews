package main

import (
	"net/http"

	"github.com/justinas/alice"
)

func (app *application) routes() http.Handler {

	mux := http.NewServeMux()
	fileServer := http.FileServer(http.Dir("./ui/static/"))
	mux.Handle("GET /static/", http.StripPrefix("/static", fileServer))

	dynamic := alice.New(app.sessionManager.LoadAndSave)

	mux.Handle("GET /{$}", dynamic.ThenFunc(app.home))
	mux.Handle("GET /profile", dynamic.ThenFunc(app.profile))
	mux.Handle("GET /movies/", dynamic.ThenFunc(app.moviesShow))           //แสดงหนังคร่าวๆ
	mux.Handle("GET /movies/{movieID}", dynamic.ThenFunc(app.movieDetail)) //แสดลงรายละเอียดหนัง มีรีวิวคร่าวๆอยู่ด้วบ
	mux.Handle("GET /movies/add", dynamic.ThenFunc(app.movieAdd))
	mux.Handle("POST /movies/add", dynamic.ThenFunc(app.movieAddPost))                 //ให้เพิ่มหนัง
	mux.Handle("GET /movies/{movieID}/review", dynamic.ThenFunc(app.movieReview))      //แสดงรีวิวทั้งหมด
	mux.Handle("POST /movies/{movieID}/review", dynamic.ThenFunc(app.movieReviewPost)) //ส่งรีวิว
	mux.Handle("GET /persons/{$}", dynamic.ThenFunc(app.personDetail))
	standard := alice.New(app.recoverPanic, app.logRequest, commonHeaders)
	// Return the 'standard' middleware chain followed by the servemux.
	
	return standard.Then(mux)

}
