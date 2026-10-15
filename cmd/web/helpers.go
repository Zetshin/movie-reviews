package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/Zetshin/movie-reviews/internal/models"
	"github.com/Zetshin/movie-reviews/internal/validator"
	"github.com/go-playground/form/v4"
	"github.com/justinas/nosurf"
)

// Cache TMDB's ordered IDs in memory, not popularity scores in MySQL.
type heroCache struct {
	mu          sync.Mutex
	ids         []int
	nextRefresh time.Time
	refreshing  bool
}

func (app *application) popularHero(ctx context.Context) ([]models.Movie, error) {
	cache := &app.heroCache
	cache.mu.Lock()
	ids := append([]int(nil), cache.ids...)
	if time.Now().Before(cache.nextRefresh) || cache.refreshing {
		cache.mu.Unlock()
		return app.movies.HeroByTMDBIDs(ids, 5)
	}
	cache.refreshing = true
	cache.mu.Unlock()
	defer func() { cache.mu.Lock(); cache.refreshing = false; cache.mu.Unlock() }()

	requestCtx, cancel := context.WithTimeout(ctx, 7*time.Second)
	defer cancel()
	var ordered []int
	seen := make(map[int]bool)
	complete := true
	// Inspect up to 100 popular movies, without importing films just for the Hero.
	for page := 1; page <= 5; page++ {
		list, err := app.tmdb.DiscoverMoviesContext(requestCtx, page)
		if err != nil {
			app.logger.Error("could not refresh popular Hero", "error", err)
			complete = false
			break
		}
		for _, movie := range list.Results {
			if movie.ID > 0 && !seen[movie.ID] {
				ordered = append(ordered, movie.ID)
				seen[movie.ID] = true
			}
		}
		if page >= list.TotalPages || len(list.Results) == 0 {
			break
		}
	}
	cache.mu.Lock()
	if complete {
		cache.ids = append([]int(nil), ordered...)
		cache.nextRefresh = time.Now().Add(30 * time.Minute)
	} else {
		// Keep the previous full list if TMDB fails; retry later rather than on every visit.
		if len(cache.ids) == 0 && len(ordered) > 0 {
			cache.ids = append([]int(nil), ordered...)
		}
		cache.nextRefresh = time.Now().Add(time.Minute)
	}
	ids = append([]int(nil), cache.ids...)
	cache.mu.Unlock()
	return app.movies.HeroByTMDBIDs(ids, 5)
}

type catalogImageUpload struct {
	image  image.Image
	format string
}

// An origin is a local movie linked to this person, never an arbitrary redirect URL.
func personOriginMovie(r *http.Request, filmography []models.FilmographyItem) models.Movie {
	id, err := strconv.Atoi(r.URL.Query().Get("from_movie"))
	if err != nil || id < 1 {
		return models.Movie{}
	}
	for _, credit := range filmography {
		if credit.ID == id {
			return credit.Movie
		}
	}
	return models.Movie{}
}

func readImageUploads(r *http.Request, v *validator.Validator, fields ...string) map[string]catalogImageUpload {
	images := make(map[string]catalogImageUpload)
	if r.MultipartForm == nil {
		return images
	}
	for _, field := range fields {
		file, header, err := r.FormFile(field + "_file")
		if errors.Is(err, http.ErrMissingFile) {
			continue
		}
		if err != nil {
			v.AddFieldError(field, "Could not read the uploaded image.")
			continue
		}
		func() {
			defer file.Close()
			if header.Size > 5<<20 {
				v.AddFieldError(field, "Choose a JPG or PNG no larger than 5 MB.")
				return
			}
			config, format, err := image.DecodeConfig(file)
			if err != nil || (format != "jpeg" && format != "png") || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > 16000000 {
				v.AddFieldError(field, "Use a JPG or PNG with no more than 16 million pixels.")
				return
			}
			if _, err := file.Seek(0, 0); err != nil {
				v.AddFieldError(field, "Could not read the uploaded image.")
				return
			}
			decoded, _, err := image.Decode(file)
			if err != nil {
				v.AddFieldError(field, "This image could not be decoded.")
				return
			}
			images[field] = catalogImageUpload{image: decoded, format: format}
		}()
	}
	return images
}

func saveImageUploads(images map[string]catalogImageUpload, section string) (map[string]string, error) {
	paths := make(map[string]string)
	if len(images) == 0 {
		return paths, nil
	}
	if section != "movies" && section != "persons" {
		return nil, errors.New("invalid catalog image directory")
	}
	directory := filepath.Join("ui/static/uploads", section)
	if err := os.MkdirAll(directory, 0755); err != nil {
		return nil, err
	}
	for field, upload := range images {
		var random [16]byte
		if _, err := rand.Read(random[:]); err != nil {
			removeImageUploads(paths)
			return nil, err
		}
		extension := ".png"
		if upload.format == "jpeg" {
			extension = ".jpg"
		}
		name := hex.EncodeToString(random[:]) + extension
		file, err := os.OpenFile(filepath.Join(directory, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			removeImageUploads(paths)
			return nil, err
		}
		paths[field] = "/static/uploads/" + section + "/" + name
		if upload.format == "jpeg" {
			err = jpeg.Encode(file, upload.image, &jpeg.Options{Quality: 90})
		} else {
			err = png.Encode(file, upload.image)
		}
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			removeImageUploads(paths)
			return nil, errors.Join(err, closeErr)
		}
	}
	return paths, nil
}

// Only clean up files generated by the current failed save, never a submitted path.
func removeImageUploads(paths map[string]string) {
	pattern := regexp.MustCompile(`^/static/uploads/(movies|persons)/([a-f0-9]{32}\.(?:jpg|png))$`)
	for _, path := range paths {
		if match := pattern.FindStringSubmatch(path); len(match) == 3 {
			os.Remove(filepath.Join("ui/static/uploads", match[1], match[2]))
		}
	}
}

func (app *application) render(w http.ResponseWriter, r *http.Request, status int, page string, data templateData) {
	ts, ok := app.templateCache[page]
	if !ok {
		err := fmt.Errorf("the template %s does not exist", page)
		app.serverError(w, r, err)
		return
	}
	// Initialize a new buffer.
	buf := new(bytes.Buffer)
	// Write the template to the buffer, instead of straight to the
	// http.ResponseWriter. If there's an error, call our serverError() helper
	// and then return.
	err := ts.ExecuteTemplate(buf, "base", data)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	// If the template is written to the buffer without any errors, it's safe
	// to go ahead and write the HTTP status code to http.ResponseWriter.

	w.WriteHeader(status)
	// Write the contents of the buffer to the http.ResponseWriter. Note: this
	// is another time where we pass our http.ResponseWriter to a function that
	// takes an io.Writer.
	buf.WriteTo(w)
}

func (app *application) serverError(w http.ResponseWriter, r *http.Request, err error) {
	var (
		method = r.Method
		uri    = r.URL.RequestURI()
	)
	app.logger.Error(err.Error(), "method", method, "uri", uri)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func (app *application) clientError(w http.ResponseWriter, status int) {
	http.Error(w, http.StatusText(status), status)
}

// Create a newTemplateData() helper, which returns a templateData struct
// initialized with the current year. Note that we're not using the *http.Request
// parameter here at the moment, but we will use it later in the book.
func (app *application) newTemplateData(r *http.Request) templateData {
	data := templateData{
		PageURL:         r.URL.RequestURI(),
		CurrentYear:     time.Now().Year(),
		Flash:           app.sessionManager.PopString(r.Context(), "flash"),
		IsAuthenticated: app.isAuthenticated(r),
		IsAdmin:         app.isAdmin(r),
		CSRFToken:       nosurf.Token(r),
	}
	if data.IsAuthenticated {
		userID := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")
		user, err := app.users.Get(userID)
		if err != nil {
			app.logger.Error("could not load navigation avatar", "error", err)
		} else {
			data.User = user
		}
	}
	return data
}

// is the target destination into which we want to decode the form data.
func (app *application) decodePostForm(r *http.Request, dst any) error {
	// Call ParseForm() on the request, in the same way that we did in our
	// snippetCreatePost handler.
	err := r.ParseForm()
	if err != nil {
		return err
	}
	// Call Decode() on our decoder instance, passing the target destination as
	// the first parameter.
	err = app.formDecoder.Decode(dst, r.PostForm)
	if err != nil {
		// If we try to use an invalid target destination, the Decode() method
		// will return an error with the type form.InvalidDecoderError. We use
		// errors.AsType() to check for this and panic. At the end of this
		// chapter we'll talk about panicking versus returning errors, and
		// discuss why it's an appropriate thing to do in this specific situation.
		if _, ok := errors.AsType[*form.InvalidDecoderError](err); ok {
			panic(err)
		}

		// For all other errors, return them as normal.
		return err
	}
	return nil
}

// Return true if the current request is from an authenticated user, otherwise
// return false.
func (app *application) isAuthenticated(r *http.Request) bool {
	if !app.sessionManager.Exists(r.Context(), "authenticatedUserID") {
		return false
	}
	id := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")
	sessionVersion := app.sessionManager.GetInt(r.Context(), "authenticatedAuthVersion")
	version, err := app.users.AuthVersion(id)
	if err != nil || sessionVersion < 1 || sessionVersion != version {
		app.sessionManager.Remove(r.Context(), "authenticatedUserID")
		app.sessionManager.Remove(r.Context(), "authenticatedAuthVersion")
		return false
	}
	return true
}

func (app *application) isAdmin(r *http.Request) bool {
	if !app.isAuthenticated(r) {
		return false
	}

	id := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")

	isAdmin, err := app.users.IsAdmin(id)
	if err != nil {
		app.logger.Error("cannot check admin status", "error", err)
		return false
	}

	return isAdmin
}

const avatarDirectory = "ui/static/uploads/avatars"

func squareAvatar(source image.Image) *image.RGBA {
	bounds := source.Bounds()
	side := min(bounds.Dx(), bounds.Dy())
	x0, y0 := bounds.Min.X+(bounds.Dx()-side)/2, bounds.Min.Y+(bounds.Dy()-side)/2
	result := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			result.Set(x, y, source.At(x0+x*side/256, y0+y*side/256))
		}
	}
	return result
}

func (app *application) renderAvatarError(w http.ResponseWriter, r *http.Request, message string) {
	data := app.newTemplateData(r)
	var err error
	userID := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")
	data.User, err = app.users.Get(userID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	data.UserReviews, err = app.reviews.ListByUserID(userID)
	if err != nil {
		app.serverError(w, r, err)
		return
	}
	data.AvatarError = message
	app.render(w, r, http.StatusUnprocessableEntity, "profile.tmpl", data)
}

func (app *application) ownedReview(w http.ResponseWriter, r *http.Request) (models.Review, bool) {
	id, err := strconv.Atoi(r.PathValue("reviewID"))
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return models.Review{}, false
	}
	userID := app.sessionManager.GetInt(r.Context(), "authenticatedUserID")
	review, err := app.reviews.GetForUser(id, userID)
	if err != nil {
		if errors.Is(err, models.ErrNoRecord) {
			http.NotFound(w, r)
		} else {
			app.serverError(w, r, err)
		}
		return models.Review{}, false
	}
	return review, true
}
