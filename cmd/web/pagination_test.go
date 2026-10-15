package main

import (
	"bytes"
	"net/url"
	"strconv"
	"testing"
)

func TestPageLinks(t *testing.T) {
	for _, tc := range []struct{ current, total int }{{1, 1}, {1, 7}, {1, 500}, {250, 500}, {500, 500}} {
		links := pageLinks(tc.current, tc.total, "/movies/add?category=search&q=John+Wick&sort=popular&page=2", "reviews")
		if links[0].Number != 1 || links[len(links)-1].Number != tc.total {
			t.Fatalf("missing first/last page: %+v", links)
		}
		selected := 0
		previous := 0
		for _, link := range links {
			if link.Number == 0 {
				continue
			}
			if link.Number <= previous || link.Number > tc.total {
				t.Fatalf("unordered/out-of-range page: %+v", links)
			}
			previous = link.Number
			u, err := url.Parse(link.URL)
			if err != nil {
				t.Fatal(err)
			}
			q := u.Query()
			if u.Path != "/movies/add" || q.Get("category") != "search" || q.Get("q") != "John Wick" || q.Get("sort") != "popular" || q.Get("page") != strconv.Itoa(link.Number) || u.Fragment != "reviews" {
				t.Fatalf("lost query or fragment: %s", link.URL)
			}
			if link.Current {
				selected++
				if link.Number != tc.current {
					t.Fatalf("wrong current page: %+v", link)
				}
			}
		}
		if selected != 1 || len(links) > 11 {
			t.Fatalf("invalid pagination window: %+v", links)
		}
	}
	if links := pageLinks(1, 0, "/movies/", ""); len(links) != 0 {
		t.Fatalf("empty catalog has page links: %+v", links)
	}
}

func TestNumberedPaginationTemplates(t *testing.T) {
	t.Chdir("../..")
	cache, err := newTemplateCache()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"movies.tmpl", "add.tmpl", "person-add.tmpl", "detail.tmpl"} {
		var output bytes.Buffer
		if err := cache[name].ExecuteTemplate(&output, "page-links", pageLinks(2, 10, "/movies/?q=test", "")); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Contains(output.Bytes(), []byte(`aria-current="page"`)) || !bytes.Contains(output.Bytes(), []byte(`page=10`)) {
			t.Fatalf("%s: missing current or last page", name)
		}
	}
}
