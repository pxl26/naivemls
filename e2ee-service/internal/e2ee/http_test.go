package e2ee

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestProposalAndSyncEndpoints(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "e2ee.txt"))
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(store)

	response := performJSON(router, http.MethodPost, "/e2ee/v1/global-room/proposal", `{"epoch":1,"data":"AQ=="}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("proposal status=%d body=%s", response.Code, response.Body.String())
	}

	response = performJSON(router, http.MethodGet, "/e2ee/v1/global-room/sync?local_epoch=0", "")
	if response.Code != http.StatusOK {
		t.Fatalf("sync status=%d body=%s", response.Code, response.Body.String())
	}
	var syncResponse struct {
		CurrentEpoch uint64   `json:"current_epoch"`
		Commits      []Commit `json:"commits"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &syncResponse); err != nil {
		t.Fatal(err)
	}
	if syncResponse.CurrentEpoch != 1 || len(syncResponse.Commits) != 1 {
		t.Fatalf("unexpected sync response: %+v", syncResponse)
	}

	response = performJSON(router, http.MethodPost, "/e2ee/v1/global-room/proposal", `{"epoch":1,"data":"AQ=="}`)
	if response.Code != http.StatusConflict {
		t.Fatalf("duplicate proposal status=%d body=%s", response.Code, response.Body.String())
	}

	response = performJSON(router, http.MethodGet, "/e2ee/v1/global-room/sync?local_epoch=2", "")
	if response.Code != http.StatusConflict {
		t.Fatalf("newer local epoch status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestProposalRejectsInvalidBase64(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "e2ee.txt"))
	if err != nil {
		t.Fatal(err)
	}
	response := performJSON(NewRouter(store), http.MethodPost, "/e2ee/v1/global-room/proposal", `{"epoch":1,"data":"not base64"}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func performJSON(handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
