package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dairygate/raw-milk-tank-intake-inspection/catalog"
	"github.com/dairygate/raw-milk-tank-intake-inspection/service"
	"github.com/dairygate/raw-milk-tank-intake-inspection/store"
)

func TestModel_OccupancyTaskIDComesOnlyFromPath(t *testing.T) {
	type occupancyView struct {
		TaskID string `json:"taskId"`
	}
	type occupancyResult struct {
		TaskID   string          `json:"taskId"`
		Status   string          `json:"status"`
		Acquired []occupancyView `json:"acquired"`
	}
	type snapshotView struct {
		Task struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"task"`
		Occupancies []occupancyView `json:"occupancies"`
	}

	cases := []struct {
		name       string
		bodyTaskID string
	}{
		{name: "empty_body_task_id", bodyTaskID: ""},
		{name: "matching_body_task_id", bodyTaskID: "$path"},
		{name: "foreign_body_task_id", bodyTaskID: "$other"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st := store.NewMemoryStore(catalog.NewFixedCatalog())
			t.Cleanup(func() { _ = st.Close() })
			h := NewHandler(service.NewService(st, service.SystemClock{}))

			doJSON := func(method, path string, body any) *httptest.ResponseRecorder {
				t.Helper()
				var buf bytes.Buffer
				if body != nil {
					if err := json.NewEncoder(&buf).Encode(body); err != nil {
						t.Fatal(err)
					}
				}
				req := httptest.NewRequest(method, path, &buf)
				req.Header.Set("Content-Type", "application/json")
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				return rec
			}
			requireStatus := func(rec *httptest.ResponseRecorder, want int) {
				t.Helper()
				if rec.Code != want {
					t.Fatalf("status = %d, want %d (%s)", rec.Code, want, rec.Body.String())
				}
			}
			createTask := func(id, batch string) {
				t.Helper()
				rec := doJSON(http.MethodPost, "/api/tasks", map[string]any{
					"taskId":        id,
					"farmId":        catalog.FixedFarmID,
					"tankBatch":     batch,
					"compartments":  []string{"A", "B"},
					"seals":         []string{"seal-0001", "seal-0002"},
					"recorderModel": "recorder-x1",
					"ruleVersion":   catalog.FixedRuleVersion,
					"reviewers":     []string{string(catalog.FixedReviewerA), string(catalog.FixedReviewerB)},
				})
				requireStatus(rec, http.StatusCreated)
			}
			advanceToOccupancy := func(id, batch, suffix string) {
				t.Helper()
				for i, sampler := range []catalog.PersonID{catalog.FixedSamplerA, catalog.FixedSamplerB} {
					rec := doJSON(http.MethodPost, "/api/tasks/"+id+"/sampling-confirmations", map[string]any{
						"operationId":  "op-sample-" + suffix + string(rune('a'+i)),
						"person":       sampler,
						"farmId":       catalog.FixedFarmID,
						"tankBatch":    batch,
						"compartments": []string{"A", "B"},
						"seals":        []string{"seal-0001", "seal-0002"},
						"generation":   1,
					})
					requireStatus(rec, http.StatusOK)
				}
				rec := doJSON(http.MethodPost, "/api/tasks/"+id+"/blind-splits", map[string]any{
					"operationId": "op-split-" + suffix,
					"generation":  1,
					"codes":       []string{"BCODE-" + suffix + "-A", "BCODE-" + suffix + "-B"},
				})
				requireStatus(rec, http.StatusOK)
			}
			getSnapshot := func(id string) snapshotView {
				t.Helper()
				rec := doJSON(http.MethodGet, "/api/tasks/"+id, nil)
				requireStatus(rec, http.StatusOK)
				var snap snapshotView
				if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
					t.Fatal(err)
				}
				return snap
			}

			suffix := tc.name
			pathTaskID := "task-b-" + suffix
			otherTaskID := "task-a-" + suffix
			createTask(otherTaskID, "BATCH-A-"+suffix)
			createTask(pathTaskID, "BATCH-B-"+suffix)
			advanceToOccupancy(pathTaskID, "BATCH-B-"+suffix, suffix)

			bodyTaskID := tc.bodyTaskID
			switch bodyTaskID {
			case "$path":
				bodyTaskID = pathTaskID
			case "$other":
				bodyTaskID = otherTaskID
			}

			rec := doJSON(http.MethodPost, "/api/tasks/"+pathTaskID+"/occupancies", map[string]any{
				"operationId": "op-occ-" + suffix,
				"generation":  1,
				"occupancies": []map[string]any{
					{
						"taskId":       bodyTaskID,
						"resourceType": "plate_well",
						"plateId":      "plate-" + suffix,
						"well":         "A1",
						"startAt":      0,
						"endAt":        3600,
					},
					{
						"taskId":       bodyTaskID,
						"resourceType": "incubator",
						"incubatorId":  "inc-" + suffix,
						"startAt":      0,
						"endAt":        3600,
					},
				},
			})
			requireStatus(rec, http.StatusOK)

			var got occupancyResult
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.TaskID != pathTaskID {
				t.Fatalf("result taskId = %q, want path task %q", got.TaskID, pathTaskID)
			}
			if got.Status != "cold_chain_verifying" {
				t.Fatalf("result status = %q, want cold_chain_verifying", got.Status)
			}
			if len(got.Acquired) != 2 {
				t.Fatalf("acquired count = %d, want 2: %+v", len(got.Acquired), got.Acquired)
			}
			for i, occ := range got.Acquired {
				if occ.TaskID != pathTaskID {
					t.Fatalf("acquired[%d].taskId = %q, want path task %q", i, occ.TaskID, pathTaskID)
				}
			}

			pathSnap := getSnapshot(pathTaskID)
			if pathSnap.Task.Status != "cold_chain_verifying" {
				t.Fatalf("path task status = %q, want cold_chain_verifying", pathSnap.Task.Status)
			}
			if len(pathSnap.Occupancies) != 2 {
				t.Fatalf("path task occupancy count = %d, want 2: %+v", len(pathSnap.Occupancies), pathSnap.Occupancies)
			}
			for i, occ := range pathSnap.Occupancies {
				if occ.TaskID != pathTaskID {
					t.Fatalf("path snapshot occupancy[%d].taskId = %q, want %q", i, occ.TaskID, pathTaskID)
				}
			}

			otherSnap := getSnapshot(otherTaskID)
			if len(otherSnap.Occupancies) != 0 {
				t.Fatalf("other task unexpectedly has occupancies: %+v", otherSnap.Occupancies)
			}
		})
	}
}
