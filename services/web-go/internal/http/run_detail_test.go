package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/cartridge/web/internal/orchestrator"
)

func TestGetRunHandler(t *testing.T) {
	run := orchestrator.Run{ID: "run-123", Status: "running", Owner: "qa", CreatedAt: time.Now()}
	cases := []struct {
		name   string
		client orchestrator.Client
		want   int
		pathID string
		runID  string
	}{
		{
			name: "ok",
			client: mockClient{get: func(ctx context.Context, id string) (orchestrator.Run, error) {
				if id != run.ID {
					t.Fatalf("unexpected id %s", id)
				}
				return run, nil
			}},
			want:   http.StatusOK,
			pathID: run.ID,
			runID:  run.ID,
		},
		{
			name: "not found",
			client: mockClient{get: func(context.Context, string) (orchestrator.Run, error) {
				return orchestrator.Run{}, orchestrator.ErrRunNotFound
			}},
			want:   http.StatusNotFound,
			pathID: "missing",
			runID:  run.ID,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHandler(t, tc.client)
			router := chi.NewRouter()
			router.Get("/api/v1/runs/{id}", h.instrument("runs.detail", h.getRun))

			req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/"+tc.pathID, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.want {
				t.Fatalf("expected status %d, got %d", tc.want, rec.Code)
			}

			if tc.want == http.StatusOK {
				var payload orchestrator.Run
				if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
					t.Fatalf("decode response: %v", err)
				}
				if payload.ID != tc.runID {
					t.Fatalf("expected run id %s, got %s", tc.runID, payload.ID)
				}
			}
		})
	}
}
