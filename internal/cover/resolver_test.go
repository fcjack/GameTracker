package cover

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTryURLsPrefersFirstAvailable(t *testing.T) {
	const steamBody = "steam-cover-bytes-long-enough-for-image-detect-and-validation-check"
	const igdbBody = "igdb-cover-bytes-long-enough-for-image-detect-and-validation-check"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/steam/library.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write([]byte(steamBody))
		case "/igdb/cover.jpg":
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write([]byte(igdbBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	data, mime, sourceURL, err := tryURLs(server.Client(),
		server.URL+"/steam/library.jpg",
		server.URL+"/igdb/cover.jpg",
	)
	if err != nil {
		t.Fatalf("tryURLs() error = %v", err)
	}
	if string(data) != steamBody {
		t.Errorf("tryURLs() source = %q, want Steam cover", sourceURL)
	}
	if mime != "image/jpeg" {
		t.Errorf("tryURLs() mime = %q, want image/jpeg", mime)
	}
}

func TestTryURLsFallsBackWhenEarlierURLsMissing(t *testing.T) {
	const igdbBody = "igdb-cover-bytes-long-enough-for-image-detect-and-validation-check"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/igdb/cover.jpg" {
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write([]byte(igdbBody))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	data, _, sourceURL, err := tryURLs(server.Client(),
		server.URL+"/missing/library.jpg",
		server.URL+"/missing/header.jpg",
		server.URL+"/igdb/cover.jpg",
	)
	if err != nil {
		t.Fatalf("tryURLs() error = %v", err)
	}
	if string(data) != igdbBody {
		t.Fatalf("tryURLs() source = %q, want IGDB fallback", sourceURL)
	}
}

func TestTryURLsRejectsNonImage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("not an image but long enough to pass size checks easily"))
	}))
	defer server.Close()

	_, _, _, err := tryURLs(server.Client(), server.URL)
	if err == nil {
		t.Fatal("tryURLs() expected error for non-image content")
	}
}
