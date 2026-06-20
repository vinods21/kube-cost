package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCommandRequiresTenantHeader(t *testing.T) {
	t.Parallel()
	api := NewAPI(&fakeRepository{}, fixedNow)
	response := httptest.NewRecorder()
	api.Routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/recommendations/rec-1/approve", strings.NewReader(`{}`)))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
}

func TestApproveAppliesWorkflowCommand(t *testing.T) {
	t.Parallel()
	repository := &fakeRepository{
		result: WorkflowResult{
			Recommendation: Recommendation{TenantID: "tenant-a", RecommendationID: "rec-1", Status: "approved", Version: 2},
			Action:         ActionReference{TenantID: "tenant-a", RecommendationID: "rec-1", ActionID: "action-1", Action: "approve", Status: "recorded", OccurredAt: fixedNow()},
		},
	}
	api := NewAPI(repository, fixedNow)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/recommendations/rec-1/approve", strings.NewReader(`{"actor_id":"user-1","reason":"safe","expected_version":1}`))
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if repository.command.TenantID != "tenant-a" ||
		repository.command.RecommendationID != "rec-1" ||
		repository.command.Action != "approve" ||
		repository.command.NextStatus != "approved" ||
		repository.command.ActorID != "user-1" ||
		repository.command.Reason != "safe" ||
		repository.command.ExpectedVersion != 1 ||
		!repository.command.OccurredAt.Equal(fixedNow()) {
		t.Fatalf("command = %#v", repository.command)
	}
	var body WorkflowResult
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Recommendation.Status != "approved" || body.Action.Action != "approve" {
		t.Fatalf("body = %#v", body)
	}
}

func TestExecuteRequestsPolicyGatedExecution(t *testing.T) {
	t.Parallel()
	repository := &fakeRepository{
		result: WorkflowResult{
			Recommendation: Recommendation{TenantID: "tenant-a", RecommendationID: "rec-1", Status: "executing", Version: 2},
			Action:         ActionReference{TenantID: "tenant-a", RecommendationID: "rec-1", ActionID: "action-1", Action: "request_execution", ExecutionID: "exec-1", Status: "handoff_requested", OccurredAt: fixedNow()},
			ExecutionRequest: &ExecutionRequest{
				ExecutionID:      "exec-1",
				TenantID:         "tenant-a",
				RecommendationID: "rec-1",
				ActionID:         "action-1",
				TargetKind:       "container",
				TargetUID:        "pod/container",
				RequestedAt:      fixedNow(),
				Status:           "pending_executor",
			},
		},
	}
	api := NewAPI(repository, fixedNow)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/recommendations/rec-1/execute", strings.NewReader(`{}`))
	request.Header.Set(tenantHeader, "tenant-a")
	response := httptest.NewRecorder()

	api.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if repository.command.Action != "request_execution" || repository.command.NextStatus != "executing" {
		t.Fatalf("command = %#v", repository.command)
	}
	var body WorkflowResult
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Action.ExecutionID != "exec-1" || body.ExecutionRequest == nil || body.ExecutionRequest.Status != "pending_executor" {
		t.Fatalf("body = %#v", body)
	}
}

func TestCommandMapsConflicts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		code int
	}{
		{name: "not found", err: ErrRecommendationNotFound, code: http.StatusNotFound},
		{name: "version conflict", err: ErrVersionConflict, code: http.StatusConflict},
		{name: "invalid transition", err: ErrInvalidTransition, code: http.StatusConflict},
		{name: "unknown", err: errors.New("down"), code: http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			api := NewAPI(&fakeRepository{err: test.err}, fixedNow)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/recommendations/rec-1/reject", strings.NewReader(`{}`))
			request.Header.Set(tenantHeader, "tenant-a")
			response := httptest.NewRecorder()
			api.Routes().ServeHTTP(response, request)
			if response.Code != test.code {
				t.Fatalf("status = %d, want %d", response.Code, test.code)
			}
		})
	}
}

type fakeRepository struct {
	command WorkflowCommand
	result  WorkflowResult
	err     error
	pingErr error
}

func (r *fakeRepository) ApplyCommand(_ context.Context, command WorkflowCommand) (WorkflowResult, error) {
	r.command = command
	return r.result, r.err
}

func (r *fakeRepository) Ping(context.Context) error { return r.pingErr }

func (r *fakeRepository) Close() error { return nil }

func fixedNow() time.Time {
	return time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
}
