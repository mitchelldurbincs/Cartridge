package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/cartridge/orchestrator/internal/db"
	"github.com/cartridge/orchestrator/pkg/types"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock repository
type MockRunRepository struct {
	mock.Mock
}

func (m *MockRunRepository) GetByID(id uuid.UUID) (*models.Run, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.Run), args.Error(1)
}

func (m *MockRunRepository) UpdateHeartbeat(runID uuid.UUID, heartbeat *db.HeartbeatData) error {
	args := m.Called(runID, heartbeat)
	return args.Error(0)
}

func TestHeartbeatHandler_HandleHeartbeat(t *testing.T) {
	tests := []struct {
		name           string
		runID          string
		requestBody    interface{}
		setupMock      func(*MockRunRepository)
		expectedStatus int
		expectedError  string
	}{
		{
			name:  "valid heartbeat",
			runID: uuid.New().String(),
			requestBody: HeartbeatRequest{
				RunID:             uuid.New().String(),
				Status:            types.RuntimeStatusRunning,
				Step:              1000,
				SamplesPerSec:     100.5,
				Loss:              0.25,
				CheckpointVersion: 10,
			},
			setupMock: func(mock *MockRunRepository) {
				run := &models.Run{
					ID:           uuid.New(),
					State:        types.RunStateRunning,
					CurrentStep:  &[]int64{999}[0],
					HealthStatus: types.RunHealthHealthy,
				}
				mock.On("GetByID", mock.AnythingOfType("uuid.UUID")).Return(run, nil)
				mock.On("UpdateHeartbeat", mock.AnythingOfType("uuid.UUID"), mock.AnythingOfType("*db.HeartbeatData")).Return(nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:  "invalid run ID",
			runID: "invalid-uuid",
			requestBody: HeartbeatRequest{
				Status: types.RuntimeStatusRunning,
				Step:   1000,
			},
			setupMock:      func(mock *MockRunRepository) {},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:  "step regression",
			runID: uuid.New().String(),
			requestBody: HeartbeatRequest{
				RunID:             uuid.New().String(),
				Status:            types.RuntimeStatusRunning,
				Step:              500, // Lower than current step
				SamplesPerSec:     100.5,
				Loss:              0.25,
				CheckpointVersion: 10,
			},
			setupMock: func(mock *MockRunRepository) {
				run := &models.Run{
					ID:           uuid.New(),
					State:        types.RunStateRunning,
					CurrentStep:  &[]int64{1000}[0], // Higher than request
					HealthStatus: types.RunHealthHealthy,
				}
				mock.On("GetByID", mock.AnythingOfType("uuid.UUID")).Return(run, nil)
			},
			expectedStatus: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRepo := new(MockRunRepository)
			tt.setupMock(mockRepo)

			logger := zerolog.New(nil).Level(zerolog.Disabled)
			handler := NewHeartbeatHandler(mockRepo, logger)

			// Create request
			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(http.MethodPost, "/runs/"+tt.runID+"/heartbeat", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")

			// Add URL param
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("id", tt.runID)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			// Execute
			rr := httptest.NewRecorder()
			handler.HandleHeartbeat(rr, req)

			// Assert
			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedError != "" {
				var response map[string]interface{}
				json.Unmarshal(rr.Body.Bytes(), &response)
				assert.Contains(t, response["error"].(map[string]interface{})["message"], tt.expectedError)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}