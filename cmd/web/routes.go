package main

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/justinas/alice"
)

func (app *application) routes() http.Handler {

	mux := http.NewServeMux()
	fileServer := http.FileServer(neuteredFileSystem{http.Dir("./ui/static/")})
	mux.Handle("GET /static/", http.StripPrefix("/static", fileServer))
	mux.HandleFunc("GET /healthz", app.healthz)

	dynamic := alice.New(app.sessionManager.LoadAndSave, app.preventCSRF)
	// Plain forms are capped at 1 MB; upload routes set their own larger limit.
	forms := alice.New(limitFormBody).Extend(dynamic)
	// Login and signup run bcrypt, so throttle them per client IP.
	auth := alice.New(app.rateLimit(app.authLimiter)).Extend(forms)

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
	standard := alice.New(app.recoverPanic, app.logRequest, app.commonHeaders)

	mux.Handle("GET /user/signup", dynamic.ThenFunc(app.userSignup))
	mux.Handle("POST /user/signup", auth.ThenFunc(app.userSignupPost))
	mux.Handle("GET /user/login", dynamic.ThenFunc(app.userLogin))
	mux.Handle("POST /user/login", auth.ThenFunc(app.userLoginPost))
	mux.Handle("GET /user/forgot-password", dynamic.ThenFunc(app.forgotPassword))
	mux.Handle("POST /user/forgot-password", auth.ThenFunc(app.forgotPasswordPost))
	mux.Handle("GET /user/reset-password", dynamic.ThenFunc(app.resetPassword))
	mux.Handle("POST /user/reset-password", auth.ThenFunc(app.resetPasswordPost))

	protected := dynamic.Append(app.requireAuthenticated)
	protectedForms := forms.Append(app.requireAuthenticated)
	mux.Handle("POST /user/logout", protectedForms.ThenFunc(app.userLogoutPost))
	mux.Handle("GET /profile", protected.ThenFunc(app.profile))
	mux.Handle("POST /profile/avatar", alice.New(limitAvatarUpload).Extend(protected).ThenFunc(app.profileAvatarPost))
	mux.Handle("GET /reviews/{reviewID}/edit", protected.ThenFunc(app.reviewEdit))
	mux.Handle("POST /reviews/{reviewID}/edit", protectedForms.ThenFunc(app.reviewEditPost))
	mux.Handle("GET /movies/{movieID}/review", protected.ThenFunc(app.movieReview))
	mux.Handle("POST /movies/{movieID}/review", protectedForms.ThenFunc(app.movieReviewPost)) //ส่งรีวิว
	mux.Handle("POST /reviews/{reviewID}/like", protectedForms.ThenFunc(app.reviewLikePost))

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

// healthz reports whether the process can serve traffic. It checks the
// database because every page depends on it, but never reveals the error.
func (app *application) healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if app.db != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := app.db.PingContext(ctx); err != nil {
			app.logger.Error("health check: database unavailable", "error", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("database unavailable\n"))
			return
		}
	}
	w.Write([]byte("ok\n"))
}

func redirectLegacyPersonRoute(w http.ResponseWriter, r *http.Request) {
	target := "/actors" + strings.TrimPrefix(r.URL.Path, "/persons")
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusPermanentRedirect)
}
