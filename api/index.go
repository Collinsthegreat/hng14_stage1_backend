package handler

import (
	"net/http"
	"sync"

	"github.com/Collinsthegreat/hng14_stage1_backend/internal/bootstrap"
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
