package main

import (
	"html/template"
	"net/url"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Zetshin/movie-reviews/internal/models"
	"github.com/Zetshin/movie-reviews/internal/tmdb"
)

type paginationData struct {
	Page         int
	TotalPages   int
	TotalReviews int
	Sort         string
	PreviousPage int
	NextPage     int
}

type HomeTopPick struct {
	Movie         models.Movie
	ReviewPreview *models.Review
}

// Define a templateData type to act as the holding structure for
// any dynamic data that we want to pass to our HTML templates.

type templateData struct {
	Cast            []models.CastMember
	Person          models.Person
	PersonMovies    []models.FilmographyItem
	User            models.User
	UserReviews     []models.UserReview
	EditingReviewID int
	UserReviewID    int
	AvatarError     string
	CurrentYear     int
	Movie           models.Movie
	Movies          []models.Movie
	HeroMovies      []models.Movie
	Form            any
	Flash           string
	Reviews         []models.Review
	Pagination      paginationData
	PageURL         string
	IsAuthenticated bool // Add an IsAuthenticated field to the templateData struct.
	IsAdmin         bool
	CSRFToken       string
	TopPicks        []HomeTopPick
	Actors          []models.Person
	TMDBSearch      string
	TMDBMovies      []tmdb.Movie
	TMDBSearchDone  bool
	TMDBError       string
	TMDBCategory    string
	TMDBPage        int
	TMDBTotalPages  int
	TMDBPreviousURL string
	TMDBNextURL     string
	TMDBImported    map[int]bool
	TMDBPeople      []tmdb.TrendingPerson
	AdminSection    string
	ReturnMovie     models.Movie
}

// humanDate formats a timestamp for visitors. Timestamps are stored in UTC,
// so convert them to the server's display zone (TZ, e.g. Asia/Bangkok).
func humanDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(time.Local).Format("02 Jan 2006 at 15:04")
}

// Initialize a template.FuncMap value and store it in a global variable. This is
// essentially a string-keyed map which acts as a lookup table mapping names to
// functions.
var functions = template.FuncMap{
	"humanDate":    humanDate,
	"tmdbImageURL": tmdb.ImageURL,
	"pageLinks":    pageLinks,
}

type pageLink struct {
	Number  int
	URL     string
	Current bool
}

// Show the first/last pages and nearby pages, using zero for an ellipsis.
// Change only page, preserving searches, filters, sorting and the fragment.
func pageLinks(current, total int, rawURL, fragment string) []pageLink {
	if total < 1 {
		return nil
	}
	current = max(1, min(current, total))
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil
	}
	u.Fragment = fragment
	query := u.Query()
	var links []pageLink
	previous := 0
	for number := 1; number <= total; number++ {
		if total > 7 && number != 1 && number != total && (number < current-2 || number > current+2) {
			continue
		}
		if number-previous > 1 {
			if number-previous == 2 {
				query.Set("page", strconv.Itoa(previous+1))
				u.RawQuery = query.Encode()
				links = append(links, pageLink{Number: previous + 1, URL: u.String()})
			} else {
				links = append(links, pageLink{})
			}
		}
		query.Set("page", strconv.Itoa(number))
		u.RawQuery = query.Encode()
		links = append(links, pageLink{Number: number, URL: u.String(), Current: number == current})
		previous = number
	}
	return links
}

func newTemplateCache() (map[string]*template.Template, error) {
	// Initialize a new map to act as the cache.
	cache := map[string]*template.Template{}
	// Use the filepath.Glob() function to get a slice of all filepaths that
	// match the pattern "./ui/html/pages/*.tmpl". This will essentially give
	// us a slice of all the filepaths for our application 'page' templates
	// like [ui/html/pages/home.tmpl ui/html/pages/view.tmpl]
	pages, err := filepath.Glob("./ui/html/pages/*.tmpl")
	if err != nil {
		return nil, err
	}
	// Loop through the page filepaths one-by-one.
	for _, page := range pages {
		// Extract the file name (like 'home.tmpl') from the full filepath
		// and assign it to the name variable.
		name := filepath.Base(page)
		// Create a slice containing the filepaths for our base template, any
		// partials and the page.
		// Parse the base template file into a template set.
		ts, err := template.New(name).Funcs(functions).ParseFiles("./ui/html/base.tmpl")
		if err != nil {
			return nil, err
		}

		// Call ParseGlob() *on this template set* to add any partials.
		ts, err = ts.ParseGlob("./ui/html/partials/*.tmpl")
		if err != nil {
			return nil, err
		}

		// Parse the files into a template set.
		//เอาฟล์แต่ละหน้ามาใส่
		ts, err = ts.ParseFiles(page)
		if err != nil {
			return nil, err
		}
		// Add the template set to the map, using the name of the page
		// (like 'home.tmpl') as the key.
		cache[name] = ts
	}
	// Return the map.
	return cache, nil
}
