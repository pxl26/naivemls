package mls

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSyncCommitsDecodesPayload(t *testing.T) {
	client := NewClient("http://e2ee.test", "http://message.test")
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/e2ee/v1/global-room/sync" || request.URL.Query().Get("local_epoch") != "2" {
			t.Fatalf("unexpected request URL: %s", request.URL)
		}
		return jsonResponse(http.StatusOK, `{"current_epoch":3,"commits":[{"epoch":3,"data":"AQI="}]}`), nil
	})}

	result, err := client.SyncCommits(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if result.CurrentEpoch != 3 || len(result.Commits) != 1 || string(result.Commits[0].Data) != string([]byte{1, 2}) {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestSendMessageReturnsEpochMismatch(t *testing.T) {
	client := NewClient("http://e2ee.test", "http://message.test")
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusConflict, `{"error":"epoch_mismatch","message":"stale","current_epoch":4}`), nil
	})}

	_, err := client.SendMessage(context.Background(), 3, "alice", []byte("ciphertext"))
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %v", err)
	}
	if apiErr.StatusCode != http.StatusConflict || apiErr.CurrentEpoch == nil || *apiErr.CurrentEpoch != 4 {
		t.Fatalf("unexpected API error: %+v", apiErr)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
