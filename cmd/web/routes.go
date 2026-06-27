package main

import "net/http"

func (app *application) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", app.home)
	mux.HandleFunc("GET /profile", app.profile)
	mux.HandleFunc("GET /movies/", app.moviesShow)
	mux.HandleFunc("GET /movies/{movieID}", app.movieDetail)             //แสดลงรายละเอียดหนัง มีรีวิวคร่าวๆอยู่ด้วบ
	mux.HandleFunc("GET /movies/{movieID}/review", app.movieReview)      //แสดงรีวิวทั้งหมด
	mux.HandleFunc("POST /movies/{movieID}/review", app.movieReviewPost) //ส่งรีวิว

	return mux

}
