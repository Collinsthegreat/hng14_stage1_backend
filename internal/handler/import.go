package handler

import (
	"context"
	"encoding/csv"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/Collinsthegreat/hng14_stage1_backend/internal/model"
	"github.com/Collinsthegreat/hng14_stage1_backend/internal/service"
	"github.com/Collinsthegreat/hng14_stage1_backend/pkg/response"
)

const (
	maxFileSize = 500 * 1024 * 1024
	chunkSize   = 1000
	maxWorkers  = 4
)

var (
	importSemOnce sync.Once
	importSem     chan struct{}
)

func importWorkerCount() int {
	n, err := strconv.Atoi(os.Getenv("IMPORT_WORKERS"))
	if err != nil || n <= 0 {
		return maxWorkers
	}
	if n > 16 {
		return 16
	}
	return n
}

func importSemaphore() chan struct{} {
	importSemOnce.Do(func() {
		importSem = make(chan struct{}, importWorkerCount())
	})
	return importSem
}

func (h *ProfileHandler) ImportCSV(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFileSize)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		response.Error(w, http.StatusBadRequest, "file too large or invalid form")
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		response.Error(w, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.ReuseRecord = true
	reader.LazyQuotes = true

	header, err := reader.Read()
	if err == io.EOF {
		response.JSON(w, http.StatusOK, service.NewImportStats().ToResponse())
		return
	}
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid CSV")
		return
	}

	colIndex, err := service.ParseProfileImportHeader(header)
	if err != nil {
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	rowCh := make(chan []string, chunkSize*2)
	stats := service.NewImportStats()

	go func() {
		defer close(rowCh)
		for {
			record, err := reader.Read()
			if err == io.EOF {
				return
			}
			if err != nil {
				atomic.AddInt64(&stats.TotalRows, 1)
				atomic.AddInt64(&stats.Skipped, 1)
				stats.AddReason("malformed_csv")
				continue
			}
			row := make([]string, len(record))
			copy(row, record)
			rowCh <- row
		}
	}()

	var wg sync.WaitGroup
	sem := importSemaphore()
	batch := make([]model.ProfileRow, 0, chunkSize)

	insertBatch := func(rows []model.ProfileRow) {
		defer wg.Done()
		defer func() { <-sem }()

		inserted, dupCount, err := h.svc.BatchInsert(r.Context(), rows)
		atomic.AddInt64(&stats.Inserted, int64(inserted))
		if dupCount > 0 {
			atomic.AddInt64(&stats.Skipped, int64(dupCount))
			stats.AddReasonN("duplicate_name", dupCount)
		}
		if err != nil {
			failed := len(rows) - inserted - dupCount
			if failed > 0 {
				atomic.AddInt64(&stats.Skipped, int64(failed))
				stats.AddReasonN("batch_error", failed)
			}
			slog.Error("batch insert partial failure", "error", err)
		}
	}

	for raw := range rowCh {
		atomic.AddInt64(&stats.TotalRows, 1)
		row, skipReason, err := service.ValidateProfileImportRow(raw, colIndex)
		if err != nil {
			atomic.AddInt64(&stats.Skipped, 1)
			stats.AddReason(skipReason)
			continue
		}
		batch = append(batch, row)

		if len(batch) >= chunkSize {
			toInsert := batch
			batch = make([]model.ProfileRow, 0, chunkSize)
			sem <- struct{}{}
			wg.Add(1)
			go insertBatch(toInsert)
		}
	}

	if len(batch) > 0 {
		toInsert := batch
		sem <- struct{}{}
		wg.Add(1)
		go insertBatch(toInsert)
	}

	wg.Wait()
	go h.svc.InvalidateProfileCache(context.Background())

	response.JSON(w, http.StatusOK, stats.ToResponse())
}
