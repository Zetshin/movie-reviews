package main

import (
	"net/http"

	"github.com/justinas/alice"
)

func (app *application) routes() http.Handler {

	mux := http.NewServeMux()
	fileServer := http.FileServer(http.Dir("./ui/static/"))
	mux.Handle("GET /static/", http.StripPrefix("/static", fileServer))

	dynamic := alice.New(app.sessionManager.LoadAndSave, preventCSRF)
	// โหลด session → ตรวจล็อกอิน

	// เพิ่มการตรวจแอดมิน

	mux.Handle("GET /{$}", dynamic.ThenFunc(app.home))
	mux.Handle("GET /movies/search", dynamic.ThenFunc(app.movieSearch))
	mux.Handle("GET /movies/", dynamic.ThenFunc(app.moviesShow))           //แสดงหนังคร่าวๆ
	mux.Handle("GET /movies/{movieID}", dynamic.ThenFunc(app.movieDetail)) //แสดลงรายละเอียดหนัง มีรีวิวคร่าวๆอยู่ด้วบ
	mux.Handle("GET /persons/search", dynamic.ThenFunc(app.personSearch))
	mux.Handle("GET /persons/{id}", dynamic.ThenFunc(app.personDetail))
	standard := alice.New(app.recoverPanic, app.logRequest, commonHeaders)

	mux.Handle("GET /user/signup", dynamic.ThenFunc(app.userSignup))
	mux.Handle("POST /user/signup", dynamic.ThenFunc(app.userSignupPost))
	mux.Handle("GET /user/login", dynamic.ThenFunc(app.userLogin))
	mux.Handle("POST /user/login", dynamic.ThenFunc(app.userLoginPost))

	protected := dynamic.Append(app.requireAuthenticated)
	mux.Handle("POST /user/logout", protected.ThenFunc(app.userLogoutPost))
	mux.Handle("GET /profile", protected.ThenFunc(app.profile))
	mux.Handle("GET /movies/{movieID}/review", protected.ThenFunc(app.movieReview))
	mux.Handle("POST /movies/{movieID}/review", protected.ThenFunc(app.movieReviewPost)) //ส่งรีวิว

	adminOnly := protected.Append(app.requireAdmin)
	mux.Handle("GET /movies/add", adminOnly.ThenFunc(app.movieAdd))
	mux.Handle("POST /movies/add", adminOnly.ThenFunc(app.movieAddPost)) //ให้เพิ่มหนัง
	return standard.Then(mux)

}
