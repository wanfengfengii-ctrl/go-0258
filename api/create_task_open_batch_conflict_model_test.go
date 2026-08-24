package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/api"
	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/service"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

func TestModel_CreateTaskRejectsDuplicateOpenTankBatch(t *testing.T) {
	cases := []struct {
		name            string
		firstTaskID     string
		duplicateTaskID string
		tankBatch       string
		compartments    []string
		seals           []string
	}{
		{
			name:            "same farm batch compartments and seals with different task id",
			firstTaskID:     "task-model-open-batch-a",
			duplicateTaskID: "task-model-open-batch-b",
			tankBatch:       "BATCH-MODEL-OPEN-DUPLICATE",
			compartments:    []string{"A", "B"},
			seals:           []string{"seal-0001", "seal-0002"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := store.NewMemoryStore(catalog.NewFixedCatalog())
			t.Cleanup(func() { _ = st.Close() })
			handler := api.NewHandler(service.NewService(st, service.SystemClock{}))

			postTask := func(taskID string) (int, map[string]any, string) {
				t.Helper()
				body := map[string]any{
					"taskId":        taskID,
					"farmId":        catalog.FixedFarmID,
					"tankBatch":     tc.tankBatch,
					"compartments":  tc.compartments,
					"seals":         tc.seals,
					"recorderModel": "recorder-x1",
					"ruleVersion":   catalog.FixedRuleVersion,
					"reviewers":     []string{"person-reviewer-a", "person-reviewer-b"},
				}
				var buf bytes.Buffer
				if err := json.NewEncoder(&buf).Encode(body); err != nil {
					t.Fatal(err)
				}
				req := httptest.NewRequest(http.MethodPost, "/api/tasks", &buf)
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				var decoded map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &decoded); err != nil {
					t.Fatalf("decode POST response for %s: %v; body=%s", taskID, err, rec.Body.String())
				}
				return rec.Code, decoded, rec.Body.String()
			}

			status, _, raw := postTask(tc.firstTaskID)
			if status != http.StatusCreated {
				t.Fatalf("first create status = %d, want 201; body=%s", status, raw)
			}

			status, body, raw := postTask(tc.duplicateTaskID)
			if status != http.StatusConflict {
				t.Fatalf("duplicate create status = %d, want 409; body=%s", status, raw)
			}
			if body["error"] != "conflict" {
				t.Fatalf("duplicate create error = %v, want conflict; body=%s", body["error"], raw)
			}

			req := httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("list tasks status = %d, want 200; body=%s", rec.Code, rec.Body.String())
			}
			var listed struct {
				Tasks []struct {
					ID        string `json:"id"`
					TankBatch string `json:"tankBatch"`
					Status    string `json:"status"`
				} `json:"tasks"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
				t.Fatalf("decode list response: %v; body=%s", err, rec.Body.String())
			}

			openTasks := 0
			for _, task := range listed.Tasks {
				if task.TankBatch == tc.tankBatch && task.Status == "pending_sampling" {
					openTasks++
					if task.ID == tc.duplicateTaskID {
						t.Fatalf("duplicate task %s was persisted in pending_sampling", tc.duplicateTaskID)
					}
				}
			}
			if openTasks != 1 {
				t.Fatalf("pending_sampling tasks for tank batch %s = %d, want 1; tasks=%+v", tc.tankBatch, openTasks, listed.Tasks)
			}
		})
	}
}
