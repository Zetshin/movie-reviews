package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv" // New import
	"strings"
	"time"

	// New import
	"github.com/Zetshin/movie-reviews/internal/models"
	"github.com/Zetshin/movie-reviews/internal/validator"
)

// movieSearch returns up to six titles for the Home dropdown.
func (app *application) movieSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if !validator.MaxChars(query, 100) {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	type result struct {
		ID          int    `json:"id"`
		Title       string `json:"title"`
		ReleaseYear string `json:"release_year"`
		PosterImage string `json:"poster_image"`
		AvgRating   string `json:"avg_rating"`
	}
	results := make([]result, 0)
	if query != "" {
		movies, err := app.movies.Search(query, 6)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		for _, movie := range movies {
			year := ""
			if !movie.ReleaseDate.IsZero() {
				year = movie.ReleaseDate.Format("2006")
			}
			results = append(results, result{movie.ID, movie.Title, year, movie.PosterImage, movie.AvgRating})
		}
	}
	payload, err := json.Marshal(results)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(payload)
}

func (app *application) home(w http.ResponseWriter, r *http.Request) {
	// หนังล่าสุด: ใช้ทั้ง Hero และ New Movies
	movies, err := app.movies.Latest(6)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// หนังที่ผ่านเกณฑ์ Top Picks
	topMovies, err := app.movies.TopPicks()
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// จับคู่หนังแต่ละเรื่องกับรีวิวของเรื่องนั้น
	var topPicks []HomeTopPick

	for _, movie := range topMovies {
		pick := HomeTopPick{
			Movie: movie,
		}

		review, err := app.reviews.GetRandom(movie.ID)
		if err != nil {
			if !errors.Is(err, models.ErrNoRecord) {
				app.serverError(w, r, err)
				return
			}
			// ไม่มีรีวิวผ่านเงื่อนไข: ReviewPreview เป็น nil
		} else {
			pick.ReviewPreview = &review
		}

		topPicks = append(topPicks, pick)
	}

	// Home ใช้ลำดับ Trending People ที่นำเข้าจาก TMDB; ข้อมูลคนเดิมยังอยู่
	actors, err := app.persons.Trending(6)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	// ส่งข้อมูลทั้งหมดให้ Home
	data := app.newTemplateData(r)
	data.Movies = movies
	data.TopPicks = topPicks
	data.Actors = actors

	app.render(w, r, http.StatusOK, "home.tmpl", data)
}
func (app *application) profile(w http.ResponseWriter, r *http.Request) {
	userID := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")
	if userID < 1 {
		http.Redirect(w, r, "/user/login", http.StatusSeeOther)
		return
	}
	user, err := app.users.Get(userID)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			app.sessionManager.Remove(r.Context(), "authenticatedUserID")
			http.Redirect(w, r, "/user/login", http.StatusSeeOther)
		} else {
			app.serverError(w, r, err)
		}
		return
	}
	data := app.newTemplateData(r)
	data.User = user
	app.render(w, r, http.StatusOK, "profile.tmpl", data)
}
func (app *application) moviesShow(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Server", "Go")
	movies, err := app.movies.Latest(10)
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
	pageSize := 10
	page := 1
	pageString := r.URL.Query().Get("page")
	if pageString != "" {
		page, err = strconv.Atoi(pageString)
		if err != nil || page < 1 {
			http.NotFound(w, r)
			return
		}
	}
	sort := r.URL.Query().Get("sort")
	totalReviews, err := app.reviews.CountByMovieID(movieID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	totalPages := (totalReviews + pageSize - 1) / pageSize
	if page < 1 || (totalPages == 0 && page != 1) || (totalPages > 0 && page > totalPages) {
		http.NotFound(w, r)
		return
	}

	offset := (page - 1) * pageSize
	reviews, err := app.reviews.ListByMovieID(movieID, sort, pageSize, offset)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data.Reviews = reviews
	data.Movie = movie
	data.Pagination.Page = page
	data.Pagination.TotalPages = totalPages
	data.Pagination.TotalReviews = totalReviews
	data.Pagination.Sort = sort
	data.Pagination.Page = page
	data.Pagination.PreviousPage = page - 1
	data.Pagination.NextPage = page + 1
	app.render(w, r, http.StatusOK, "detail.tmpl", data)

}
func (app *application) movieReview(w http.ResponseWriter, r *http.Request) {
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
	data.Form = movieReviewForm{}
	app.render(w, r, http.StatusOK, "review.tmpl", data)
}

type movieReviewForm struct {
	Rating              float64 `form:"rating"`
	ReviewText          string  `form:"review_text"`
	validator.Validator `form:"-"`
}

func (app *application) movieReviewPost(w http.ResponseWriter, r *http.Request) {
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

	var form movieReviewForm
	if err := app.decodePostForm(r, &form); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	form.ReviewText = strings.TrimSpace(form.ReviewText)
	form.CheckField(validator.PermittedValue(form.Rating, 0.5, 1.0, 1.5, 2.0, 2.5, 3.0, 3.5, 4.0, 4.5, 5.0), "rating", "Choose a rating from 0.5 to 5 in half-star increments.")
	form.CheckField(validator.NotBlank(form.ReviewText), "review_text", "Write your review before submitting.")
	form.CheckField(validator.MaxChars(form.ReviewText, 2000), "review_text", "Your review must be 2000 characters or fewer.")
	if !form.Valid() {
		data := app.newTemplateData(r)
		data.Movie = movie
		data.Form = form
		app.render(w, r, http.StatusUnprocessableEntity, "review.tmpl", data)
		return
	}

	// The author comes from the session, never from submitted form data.
	userID := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")
	if userID < 1 {
		http.Redirect(w, r, "/user/login", http.StatusSeeOther)
		return
	}
	_, err = app.reviews.Insert(movieID, userID, form.Rating, form.ReviewText)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			http.NotFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}
	app.sessionManager.Put(r.Context(), "flash", "Your review has been published.")
	http.Redirect(w, r, fmt.Sprintf("/movies/%d#reviews", movieID), http.StatusSeeOther)
}

func (app *application) movieAdd(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)

	data.Form = movieAddForm{}
	data.TMDBSearch = strings.TrimSpace(r.URL.Query().Get("q"))
	if !validator.MaxChars(data.TMDBSearch, 100) {
		data.TMDBError = "Search must be 100 characters or fewer."
		app.render(w, r, http.StatusUnprocessableEntity, "add.tmpl", data)
		return
	}
	if data.TMDBSearch != "" {
		data.TMDBSearchDone = true
		list, err := app.tmdb.SearchMovies(data.TMDBSearch)
		if err != nil {
			app.logger.Error("TMDB search failed", "error", err)
			data.TMDBError = "Could not search TMDB. Please try again."
			app.render(w, r, http.StatusBadGateway, "add.tmpl", data)
			return
		}
		data.TMDBMovies = list.Results
	}
	app.render(w, r, http.StatusOK, "add.tmpl", data)
}

type movieAddForm struct {
	Title               string `form:"title"`
	Description         string `form:"description"`
	PosterImage         string `form:"poster_image"`
	ReleaseDate         string `form:"release_date"`
	validator.Validator `form:"-"`
}

func (app *application) movieAddPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	if r.PostForm.Get("action") == "import_tmdb" {
		app.movieImportTMDBPost(w, r)
		return
	}

	// Declare a new empty instance of the snippetCreateForm struct.
	var form movieAddForm
	err := app.decodePostForm(r, &form)
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}

	form.CheckField(validator.NotBlank(form.Title), "title", "This field cannot be blank")
	form.CheckField(validator.MaxChars(form.Title, 100), "title", "This field cannot be more than 100 characters long")
	form.CheckField(validator.NotBlank(form.ReleaseDate), "release_date", "This field cannot be blank")

	if !form.Valid() {
		data := app.newTemplateData(r)
		data.Form = form
		app.render(w, r, http.StatusUnprocessableEntity, "add.tmpl", data)
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
	id, err := app.movies.Insert(form.Title, form.Description, releaseDate, form.PosterImage)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	app.sessionManager.Put(r.Context(), "flash", "Movie successfully created!")
	http.Redirect(w, r, fmt.Sprintf("/movies/%d", id), http.StatusSeeOther)
}

func (app *application) personDetail(w http.ResponseWriter, r *http.Request) {
	personID, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || personID < 1 {
		http.NotFound(w, r)
		return
	}
	person, err := app.persons.Get(personID)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			http.NotFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return
	}
	data := app.newTemplateData(r)
	data.Person = person
	data.PersonMovies, err = app.moviePersons.GetFilmography(personID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	app.render(w, r, http.StatusOK, "person.tmpl", data)
}

func (app *application) personSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if !validator.MaxChars(query, 100) {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	type result struct {
		ID           int    `json:"id"`
		Name         string `json:"name"`
		ProfileImage string `json:"profile_image"`
	}
	results := make([]result, 0)
	if query != "" {
		persons, err := app.persons.Search(query, 6)
		if err != nil {
			app.serverError(w, r, err)
			return
		}
		for _, person := range persons {
			results = append(results, result{person.ID, person.Name, person.ProfileImage})
		}
	}
	payload, err := json.Marshal(results)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Write(payload)
}

func (app *application) movieReviews(w http.ResponseWriter, r *http.Request) {
	movieID, err := strconv.Atoi(r.PathValue("movieID"))
	if err != nil || movieID < 1 {
		http.NotFound(w, r)
		return
	}
	pageSize := 10
	page := 1
	pageString := r.URL.Query().Get("page")
	if pageString != "" {
		page, err = strconv.Atoi(pageString)
		if err != nil || page < 1 {
			http.NotFound(w, r)
			return
		}
	}
	sort := r.URL.Query().Get("sort")
	totalReviews, err := app.reviews.CountByMovieID(movieID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	totalPages := (totalReviews + pageSize - 1) / pageSize
	if page < 1 || (totalPages == 0 && page != 1) || (totalPages > 0 && page > totalPages) {
		http.NotFound(w, r)
		return
	}

	offset := (page - 1) * pageSize
	reviews, err := app.reviews.ListByMovieID(movieID, sort, pageSize, offset)
	if err != nil {
		app.serverError(w, r, err)
		return
	}

	data := app.newTemplateData(r)
	data.Reviews = reviews

	app.render(w, r, http.StatusOK, "detail.tmpl", data)

}

// Create a new userSignupForm struct.
type userSignupForm struct {
	Name                string `form:"name"`
	Email               string `form:"email"`
	Password            string `form:"password"`
	validator.Validator `form:"-"`
}

// Update the handler so it displays the signup page.
func (app *application) userSignup(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	data.Form = userSignupForm{}
	app.render(w, r, http.StatusOK, "signup.tmpl", data)
}
func (app *application) userSignupPost(w http.ResponseWriter, r *http.Request) {
	// Declare a zero-valued instance of our userSignupForm struct.
	var form userSignupForm
	// Parse the form data into the userSignupForm struct.
	err := app.decodePostForm(r, &form)
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	// Validate the form contents using our helper functions.
	form.CheckField(validator.NotBlank(form.Name), "name", "This field cannot be blank")
	form.CheckField(validator.NotBlank(form.Email), "email", "This field cannot be blank")
	form.CheckField(validator.Matches(form.Email, validator.EmailRX), "email", "This field must be a valid email address")
	form.CheckField(validator.NotBlank(form.Password), "password", "This field cannot be blank")
	form.CheckField(validator.MinChars(form.Password, 8), "password", "This field must be at least 8 characters long")
	form.CheckField(validator.MaxBytes(form.Password, 72), "password", "This field must not be more than 72 bytes long")
	// If there are any errors, redisplay the signup form along with a 422
	// status code.
	if !form.Valid() {
		data := app.newTemplateData(r)
		data.Form = form
		app.render(w, r, http.StatusUnprocessableEntity, "signup.tmpl", data)
		return
	}
	// Try to create a new user record in the database. If the email already
	// exists then add an error message to the form and redisplay it.
	err = app.users.Insert(form.Name, form.Email, form.Password)
	if err != nil {
		if errors.Is(err, models.ErrDuplicateEmail) {
			form.AddFieldError("email", "Email address is already in use")
			data := app.newTemplateData(r)
			data.Form = form
			app.render(w, r, http.StatusUnprocessableEntity, "signup.tmpl", data)
		} else {
			app.serverError(w, r, err)
		}
		return
	}
	// Otherwise add a confirmation flash message to the session confirming that
	// their signup worked.
	app.sessionManager.Put(r.Context(), "flash", "Your signup was successful. Please log in.")
	// And redirect the user to the login page.
	http.Redirect(w, r, "/user/login", http.StatusSeeOther)
}

type userLoginForm struct {
	Email               string `form:"email"`
	Password            string `form:"password"`
	validator.Validator `form:"-"`
}

// Update the handler so it displays the login page.
func (app *application) userLogin(w http.ResponseWriter, r *http.Request) {
	data := app.newTemplateData(r)
	data.Form = userLoginForm{}
	app.render(w, r, http.StatusOK, "login.tmpl", data)
}

func (app *application) userLoginPost(w http.ResponseWriter, r *http.Request) {
	// Decode the form data into the userLoginForm struct.
	var form userLoginForm
	err := app.decodePostForm(r, &form)
	if err != nil {
		app.clientError(w, http.StatusBadRequest)
		return
	}
	// Do some validation checks on the form. We check that both email and
	// password are provided, and that the the format of the email address as
	// a UX-nicety (in case the user makes a typo). We also again make sure that
	// the password provided is no longer than 72 bytes to avoid any bcrypt
	// truncation issues.
	form.CheckField(validator.NotBlank(form.Email), "email", "This field cannot be blank")
	form.CheckField(validator.Matches(form.Email, validator.EmailRX), "email", "This field must be a valid email address")
	form.CheckField(validator.NotBlank(form.Password), "password", "This field cannot be blank")
	form.CheckField(validator.MaxBytes(form.Password, 72), "password", "This field must not be more than 72 bytes long")
	if !form.Valid() {
		data := app.newTemplateData(r)
		data.Form = form
		app.render(w, r, http.StatusUnprocessableEntity, "login.tmpl", data)
		return
	}
	// Check whether the credentials are valid. If they're not, add a generic
	// non-field error message and redisplay the login page.
	id, err := app.users.Authenticate(form.Email, form.Password)
	if err != nil {
		if errors.Is(err, models.ErrInvalidCredentials) {
			form.AddNonFieldError("Email or password is incorrect")
			data := app.newTemplateData(r)
			data.Form = form
			app.render(w, r, http.StatusUnprocessableEntity, "login.tmpl", data)
		} else {
			app.serverError(w, r, err)
		}
		return
	}
	// Use the RenewToken() method on the current session to change the session
	// ID. It's good practice to generate a new session ID when the
	// authentication state or privilege levels change for the user (e.g. login
	// and logout operations).
	err = app.sessionManager.RenewToken(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	// Add the ID of the current user to the session, so that they are now
	// 'logged in'.
	app.sessionManager.Put(r.Context(), "authenticatedUserID", id)
	// Redirect the user to the create snippet page.
	http.Redirect(w, r, "/movies/", http.StatusSeeOther)
}

func (app *application) userLogoutPost(w http.ResponseWriter, r *http.Request) {
	// Use the RenewToken() method on the current session to change the session
	// ID again.
	err := app.sessionManager.RenewToken(r.Context())
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	// Remove the authenticatedUserID from the session data so that the user is
	// 'logged out'.
	app.sessionManager.Remove(r.Context(), "authenticatedUserID")
	// Add a flash message to the session to confirm to the user that they've been
	// logged out.
	app.sessionManager.Put(r.Context(), "flash", "You've been logged out successfully!")
	// Redirect the user to the application home page.
	http.Redirect(w, r, "/movies/", http.StatusSeeOther)
}
