package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	mls "github.com/pxl26/naivemls/client-sdk"
)

func TestSendAndSyncMessage(t *testing.T) {
	client := &stubClient{}
	router := NewRouter(client, mls.PassthroughCodec{})

	send := request(router, http.MethodPost, "/client-sdk/v1/sendMessage", `{"epoch":2,"sender":"alice","message":"hello"}`)
	if send.Code != http.StatusCreated {
		t.Fatalf("send status=%d body=%s", send.Code, send.Body.String())
	}
	if string(client.sent.Ciphertext) != "hello" {
		t.Fatalf("unexpected ciphertext: %q", client.sent.Ciphertext)
	}

	sync := request(router, http.MethodGet, "/client-sdk/v1/syncMessage?from=0&to=999", "")
	if sync.Code != http.StatusOK {
		t.Fatalf("sync status=%d body=%s", sync.Code, sync.Body.String())
	}
	var response struct {
		Messages []struct {
			Message          string `json:"message"`
			EncryptedMessage string `json:"encrypted_message"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(sync.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Messages) != 1 || response.Messages[0].Message != "hello" || response.Messages[0].EncryptedMessage != "aGVsbG8=" {
		t.Fatalf("unexpected response: %+v", response)
	}
}

type stubClient struct {
	sent mls.Message
}

func (s *stubClient) SendMessage(_ context.Context, epoch uint64, sender string, ciphertext []byte) (mls.Message, error) {
	s.sent = mls.Message{ID: 1, Epoch: epoch, Sender: sender, Ciphertext: ciphertext, Timestamp: 100}
	return s.sent, nil
}

func (s *stubClient) SyncMessages(_ context.Context, _, _ int64) ([]mls.Message, error) {
	return []mls.Message{s.sent}, nil
}

func request(handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}
