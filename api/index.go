package handler

import (
	"net/http"
	"sync"

	"github.com/username/repo-name/internal/bootstrap"
)

var (
	once   sync.Once
	router http.Handler
)

func Handler(w http.ResponseWriter, r *http.Request) {
	once.Do(func() {
		router = bootstrap.NewRouter()
	})
	router.ServeHTTP(w, r)
}
