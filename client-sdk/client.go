package mls

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	E2EEBaseURL    string
	MessageBaseURL string
	HTTPClient     *http.Client
}

func NewClient(e2eeBaseURL, messageBaseURL string) *Client {
	return &Client{
		E2EEBaseURL:    strings.TrimRight(e2eeBaseURL, "/"),
		MessageBaseURL: strings.TrimRight(messageBaseURL, "/"),
		HTTPClient:     &http.Client{Timeout: 10 * time.Second},
	}
}

type APIError struct {
	StatusCode   int
	Code         string
	Message      string
	CurrentEpoch *uint64
}

func (e *APIError) Error() string {
	if e.CurrentEpoch != nil {
		return fmt.Sprintf("backend returned %d %s: %s (current epoch %d)", e.StatusCode, e.Code, e.Message, *e.CurrentEpoch)
	}
	return fmt.Sprintf("backend returned %d %s: %s", e.StatusCode, e.Code, e.Message)
}

type Commit struct {
	Epoch uint64
	Data  []byte
}

type CommitSync struct {
	CurrentEpoch uint64
	Commits      []Commit
}

type Message struct {
	ID         uint64
	Epoch      uint64
	Sender     string
	Ciphertext []byte
	Timestamp  int64
}

func (c *Client) SubmitProposal(ctx context.Context, epoch uint64, proposal []byte) (uint64, error) {
	request := struct {
		Epoch uint64 `json:"epoch"`
		Data  string `json:"data"`
	}{Epoch: epoch, Data: base64.StdEncoding.EncodeToString(proposal)}
	var response struct {
		CurrentEpoch uint64 `json:"current_epoch"`
	}
	err := c.doJSON(ctx, http.MethodPost, c.E2EEBaseURL+"/e2ee/v1/global-room/proposal", request, &response)
	return response.CurrentEpoch, err
}

func (c *Client) SyncCommits(ctx context.Context, localEpoch uint64) (CommitSync, error) {
	endpoint, err := url.Parse(c.E2EEBaseURL + "/e2ee/v1/global-room/sync")
	if err != nil {
		return CommitSync{}, err
	}
	query := endpoint.Query()
	query.Set("local_epoch", strconv.FormatUint(localEpoch, 10))
	endpoint.RawQuery = query.Encode()

	var response struct {
		CurrentEpoch uint64 `json:"current_epoch"`
		Commits      []struct {
			Epoch uint64 `json:"epoch"`
			Data  string `json:"data"`
		} `json:"commits"`
	}
	if err := c.doJSON(ctx, http.MethodGet, endpoint.String(), nil, &response); err != nil {
		return CommitSync{}, err
	}

	result := CommitSync{CurrentEpoch: response.CurrentEpoch, Commits: make([]Commit, 0, len(response.Commits))}
	for _, commit := range response.Commits {
		data, err := base64.StdEncoding.DecodeString(commit.Data)
		if err != nil {
			return CommitSync{}, fmt.Errorf("decode commit for epoch %d: %w", commit.Epoch, err)
		}
		result.Commits = append(result.Commits, Commit{Epoch: commit.Epoch, Data: data})
	}
	return result, nil
}

func (c *Client) SendMessage(ctx context.Context, epoch uint64, sender string, ciphertext []byte) (Message, error) {
	request := struct {
		Epoch  uint64 `json:"epoch"`
		Sender string `json:"sender"`
		Data   string `json:"data"`
	}{Epoch: epoch, Sender: sender, Data: base64.StdEncoding.EncodeToString(ciphertext)}
	var response messageWire
	if err := c.doJSON(ctx, http.MethodPost, c.MessageBaseURL+"/message/v1/global-room/message", request, &response); err != nil {
		return Message{}, err
	}
	return response.decode()
}

func (c *Client) SyncMessages(ctx context.Context, from, to int64) ([]Message, error) {
	endpoint, err := url.Parse(c.MessageBaseURL + "/message/v1/global-room/message/sync")
	if err != nil {
		return nil, err
	}
	query := endpoint.Query()
	query.Set("from", strconv.FormatInt(from, 10))
	query.Set("to", strconv.FormatInt(to, 10))
	endpoint.RawQuery = query.Encode()

	var response struct {
		Messages []messageWire `json:"messages"`
	}
	if err := c.doJSON(ctx, http.MethodGet, endpoint.String(), nil, &response); err != nil {
		return nil, err
	}
	messages := make([]Message, 0, len(response.Messages))
	for _, wire := range response.Messages {
		message, err := wire.decode()
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, nil
}

type messageWire struct {
	ID        uint64 `json:"id"`
	Epoch     uint64 `json:"epoch"`
	Sender    string `json:"sender"`
	Data      string `json:"data"`
	Timestamp int64  `json:"timestamp"`
}

func (m messageWire) decode() (Message, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(m.Data)
	if err != nil {
		return Message{}, fmt.Errorf("decode message %d: %w", m.ID, err)
	}
	return Message{ID: m.ID, Epoch: m.Epoch, Sender: m.Sender, Ciphertext: ciphertext, Timestamp: m.Timestamp}, nil
}

func (c *Client) doJSON(ctx context.Context, method, endpoint string, input, output any) error {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	response, err := httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var wire struct {
			Code         string  `json:"error"`
			Message      string  `json:"message"`
			CurrentEpoch *uint64 `json:"current_epoch"`
		}
		if err := json.NewDecoder(response.Body).Decode(&wire); err != nil {
			return fmt.Errorf("backend returned status %d", response.StatusCode)
		}
		return &APIError{StatusCode: response.StatusCode, Code: wire.Code, Message: wire.Message, CurrentEpoch: wire.CurrentEpoch}
	}
	if output == nil {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(output); err != nil {
		return fmt.Errorf("decode backend response: %w", err)
	}
	return nil
}
