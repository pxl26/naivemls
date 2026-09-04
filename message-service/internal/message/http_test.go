package message

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestMessageServiceChecksE2EEEpochAndSyncs(t *testing.T) {
	var e2eeEpoch atomic.Uint64
	e2eeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		localEpoch, _ := strconv.ParseUint(r.URL.Query().Get("local_epoch"), 10, 64)
		currentEpoch := e2eeEpoch.Load()
		status := http.StatusOK
		if localEpoch > currentEpoch {
			status = http.StatusConflict
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]uint64{"current_epoch": currentEpoch})
	}))
	defer e2eeServer.Close()

	messageStore, err := OpenStore(filepath.Join(t.TempDir(), "messages.txt"))
	if err != nil {
		t.Fatal(err)
	}
	messageStore.now = func() time.Time { return time.UnixMilli(500) }
	messageRouter := NewRouter(messageStore, NewHTTPEpochChecker(e2eeServer.URL, time.Second))

	response := performJSON(messageRouter, http.MethodPost, "/message/v1/global-room/message", `{"epoch":0,"sender":"alice","data":"AQ=="}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("initial message status=%d body=%s", response.Code, response.Body.String())
	}

	e2eeEpoch.Store(1)
	response = performJSON(messageRouter, http.MethodPost, "/message/v1/global-room/message", `{"epoch":0,"sender":"alice","data":"Aw=="}`)
	if response.Code != http.StatusConflict {
		t.Fatalf("stale message status=%d body=%s", response.Code, response.Body.String())
	}
	var mismatch struct {
		CurrentEpoch uint64 `json:"current_epoch"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &mismatch); err != nil {
		t.Fatal(err)
	}
	if mismatch.CurrentEpoch != 1 {
		t.Fatalf("expected current epoch 1, got %d", mismatch.CurrentEpoch)
	}

	response = performJSON(messageRouter, http.MethodPost, "/message/v1/global-room/message", `{"epoch":1,"sender":"bob","data":"BA=="}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("latest message status=%d body=%s", response.Code, response.Body.String())
	}

	response = performJSON(messageRouter, http.MethodGet, "/message/v1/global-room/message/sync?from=500&to=500", "")
	if response.Code != http.StatusOK {
		t.Fatalf("sync status=%d body=%s", response.Code, response.Body.String())
	}
	var syncResponse struct {
		Messages []Message `json:"messages"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &syncResponse); err != nil {
		t.Fatal(err)
	}
	if len(syncResponse.Messages) != 2 {
		t.Fatalf("expected 2 accepted messages, got %+v", syncResponse.Messages)
	}
}

func TestMessageSyncRejectsInvalidRange(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "messages.txt"))
	if err != nil {
		t.Fatal(err)
	}
	router := NewRouter(store, staticEpoch(0))
	response := performJSON(router, http.MethodGet, "/message/v1/global-room/message/sync?from=2&to=1", "")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

type staticEpoch uint64

func (e staticEpoch) CurrentEpoch(_ context.Context, _ uint64) (uint64, error) {
	return uint64(e), nil
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
