package main

import (
	"net/http"
	"strings"

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
	mux.Handle("GET /actors/{$}", dynamic.ThenFunc(app.peopleShow))
	mux.Handle("GET /actors/search", dynamic.ThenFunc(app.personSearch))
	mux.Handle("GET /actors/{id}", dynamic.ThenFunc(app.personDetail))
	mux.Handle("GET /persons/{$}", dynamic.ThenFunc(redirectLegacyPersonRoute))
	mux.Handle("GET /persons/search", dynamic.ThenFunc(redirectLegacyPersonRoute))
	mux.Handle("GET /persons/{id}", dynamic.ThenFunc(redirectLegacyPersonRoute))
	standard := alice.New(app.recoverPanic, app.logRequest, commonHeaders)

	mux.Handle("GET /user/signup", dynamic.ThenFunc(app.userSignup))
	mux.Handle("POST /user/signup", dynamic.ThenFunc(app.userSignupPost))
	mux.Handle("GET /user/login", dynamic.ThenFunc(app.userLogin))
	mux.Handle("POST /user/login", dynamic.ThenFunc(app.userLoginPost))

	protected := dynamic.Append(app.requireAuthenticated)
	mux.Handle("POST /user/logout", protected.ThenFunc(app.userLogoutPost))
	mux.Handle("GET /profile", protected.ThenFunc(app.profile))
	mux.Handle("POST /profile/avatar", alice.New(limitAvatarUpload).Extend(protected).ThenFunc(app.profileAvatarPost))
	mux.Handle("GET /reviews/{reviewID}/edit", protected.ThenFunc(app.reviewEdit))
	mux.Handle("POST /reviews/{reviewID}/edit", protected.ThenFunc(app.reviewEditPost))
	mux.Handle("GET /movies/{movieID}/review", protected.ThenFunc(app.movieReview))
	mux.Handle("POST /movies/{movieID}/review", protected.ThenFunc(app.movieReviewPost)) //ส่งรีวิว

	adminOnly := alice.New(limitMovieUpload).Extend(protected.Append(app.requireAdmin))
	mux.Handle("GET /actors/add", adminOnly.ThenFunc(app.personAdd))
	mux.Handle("POST /actors/add", adminOnly.ThenFunc(app.personAddPost))
	mux.Handle("GET /persons/add", adminOnly.ThenFunc(redirectLegacyPersonRoute))
	mux.Handle("POST /persons/add", adminOnly.ThenFunc(app.personAddPost))
	mux.Handle("POST /movies/import", adminOnly.ThenFunc(app.movieImportOnePost))
	mux.Handle("POST /movies/{movieID}/cast/{personID}/import", adminOnly.ThenFunc(app.movieCastImportPost))
	mux.Handle("GET /movies/{movieID}/edit", adminOnly.ThenFunc(app.movieEdit))
	mux.Handle("POST /movies/{movieID}/edit", adminOnly.ThenFunc(app.movieEditPost))
	mux.Handle("GET /movies/{movieID}/delete", adminOnly.ThenFunc(app.movieDelete))
	mux.Handle("POST /movies/{movieID}/delete", adminOnly.ThenFunc(app.movieDeletePost))
	mux.Handle("GET /actors/{id}/edit", adminOnly.ThenFunc(app.personEdit))
	mux.Handle("POST /actors/{id}/edit", adminOnly.ThenFunc(app.personEditPost))
	mux.Handle("GET /actors/{id}/delete", adminOnly.ThenFunc(app.personDelete))
	mux.Handle("POST /actors/{id}/delete", adminOnly.ThenFunc(app.personDeletePost))
	mux.Handle("GET /persons/{id}/edit", adminOnly.ThenFunc(redirectLegacyPersonRoute))
	mux.Handle("POST /persons/{id}/edit", adminOnly.ThenFunc(app.personEditPost))
	mux.Handle("GET /persons/{id}/delete", adminOnly.ThenFunc(redirectLegacyPersonRoute))
	mux.Handle("POST /persons/{id}/delete", adminOnly.ThenFunc(app.personDeletePost))
	mux.Handle("GET /movies/add", adminOnly.ThenFunc(app.movieAdd))
	mux.Handle("POST /movies/add", adminOnly.ThenFunc(app.movieAddPost)) //ให้เพิ่มหนัง
	return standard.Then(mux)

}

func redirectLegacyPersonRoute(w http.ResponseWriter, r *http.Request) {
	target := "/actors" + strings.TrimPrefix(r.URL.Path, "/persons")
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusPermanentRedirect)
}
