package message

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type EpochChecker interface {
	CurrentEpoch(ctx context.Context, localEpoch uint64) (uint64, error)
}

type HTTPEpochChecker struct {
	baseURL string
	client  *http.Client
}

func NewHTTPEpochChecker(baseURL string, timeout time.Duration) *HTTPEpochChecker {
	return &HTTPEpochChecker{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *HTTPEpochChecker) CurrentEpoch(ctx context.Context, localEpoch uint64) (uint64, error) {
	endpoint, err := url.Parse(c.baseURL + "/e2ee/v1/global-room/sync")
	if err != nil {
		return 0, err
	}
	query := endpoint.Query()
	query.Set("local_epoch", strconv.FormatUint(localEpoch, 10))
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return 0, err
	}
	response, err := c.client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()

	var body struct {
		CurrentEpoch uint64 `json:"current_epoch"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return 0, fmt.Errorf("decode E2EE response: %w", err)
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusConflict {
		return 0, fmt.Errorf("E2EE service returned status %d", response.StatusCode)
	}
	return body.CurrentEpoch, nil
}

type Handler struct {
	store   *Store
	checker EpochChecker
}

type sendRequest struct {
	Epoch  uint64 `json:"epoch"`
	Sender string `json:"sender"`
	Data   string `json:"data" binding:"required"`
}

func NewRouter(store *Store, checker EpochChecker) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery(), cors())
	h := &Handler{store: store, checker: checker}
	router.POST("/message/v1/global-room/message", h.send)
	router.GET("/message/v1/global-room/message/sync", h.sync)
	return router
}

func (h *Handler) send(c *gin.Context) {
	var request sendRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "epoch and base64 data are required", nil)
		return
	}
	if _, err := base64.StdEncoding.DecodeString(request.Data); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "data must be valid base64", nil)
		return
	}

	currentEpoch, err := h.checker.CurrentEpoch(c.Request.Context(), request.Epoch)
	if err != nil {
		writeError(c, http.StatusBadGateway, "e2ee_unavailable", "could not validate message epoch", nil)
		return
	}
	if request.Epoch != currentEpoch {
		writeError(c, http.StatusConflict, "epoch_mismatch", "message epoch is not the latest epoch", &currentEpoch)
		return
	}

	message, err := h.store.Add(request.Epoch, request.Sender, request.Data)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "could not persist message", nil)
		return
	}
	c.JSON(http.StatusCreated, message)
}

func (h *Handler) sync(c *gin.Context) {
	from, err := strconv.ParseInt(c.Query("from"), 10, 64)
	if err != nil || from < 0 {
		writeError(c, http.StatusBadRequest, "invalid_request", "from must be a non-negative Unix timestamp in milliseconds", nil)
		return
	}
	to, err := strconv.ParseInt(c.Query("to"), 10, 64)
	if err != nil || to < 0 {
		writeError(c, http.StatusBadRequest, "invalid_request", "to must be a non-negative Unix timestamp in milliseconds", nil)
		return
	}
	if from > to {
		writeError(c, http.StatusBadRequest, "invalid_request", "from must be less than or equal to to", nil)
		return
	}

	messages := h.store.Between(from, to)
	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

func writeError(c *gin.Context, status int, code, message string, currentEpoch *uint64) {
	body := gin.H{"error": code, "message": message}
	if currentEpoch != nil {
		body["current_epoch"] = *currentEpoch
	}
	c.JSON(status, body)
}

func cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
