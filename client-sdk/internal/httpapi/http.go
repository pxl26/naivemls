package httpapi

import (
	"context"
	"encoding/base64"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	mls "github.com/pxl26/naivemls/client-sdk"
)

type MessageClient interface {
	SendMessage(ctx context.Context, epoch uint64, sender string, ciphertext []byte) (mls.Message, error)
	SyncMessages(ctx context.Context, from, to int64) ([]mls.Message, error)
}

type Handler struct {
	client MessageClient
	codec  mls.MessageCodec
}

type sendRequest struct {
	Epoch   *uint64 `json:"epoch" binding:"required"`
	Sender  string  `json:"sender" binding:"required"`
	Message string  `json:"message" binding:"required"`
}

type messageResponse struct {
	ID               uint64 `json:"id"`
	Epoch            uint64 `json:"epoch"`
	Sender           string `json:"sender"`
	Message          string `json:"message"`
	EncryptedMessage string `json:"encrypted_message"`
	Timestamp        int64  `json:"timestamp"`
}

func NewRouter(client MessageClient, codec mls.MessageCodec) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery(), cors())
	h := &Handler{client: client, codec: codec}
	router.POST("/client-sdk/v1/sendMessage", h.sendMessage)
	router.GET("/client-sdk/v1/syncMessage", h.syncMessage)
	return router
}

func (h *Handler) sendMessage(c *gin.Context) {
	var request sendRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "epoch, sender, and message are required", nil)
		return
	}
	ciphertext, err := h.codec.Encrypt(*request.Epoch, []byte(request.Message))
	if err != nil {
		writeError(c, http.StatusInternalServerError, "encrypt_failed", "could not encrypt message", nil)
		return
	}

	message, err := h.client.SendMessage(c.Request.Context(), *request.Epoch, request.Sender, ciphertext)
	if err != nil {
		writeClientError(c, err)
		return
	}
	response, err := h.decodeMessage(message)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "decrypt_failed", "could not decrypt sent message", nil)
		return
	}
	c.JSON(http.StatusCreated, response)
}

func (h *Handler) syncMessage(c *gin.Context) {
	from, err := strconv.ParseInt(c.Query("from"), 10, 64)
	if err != nil || from < 0 {
		writeError(c, http.StatusBadRequest, "invalid_request", "from must be a non-negative Unix timestamp in milliseconds", nil)
		return
	}
	to, err := strconv.ParseInt(c.Query("to"), 10, 64)
	if err != nil || to < 0 || from > to {
		writeError(c, http.StatusBadRequest, "invalid_request", "to must be a Unix timestamp in milliseconds greater than or equal to from", nil)
		return
	}

	messages, err := h.client.SyncMessages(c.Request.Context(), from, to)
	if err != nil {
		writeClientError(c, err)
		return
	}
	responses := make([]messageResponse, 0, len(messages))
	for _, message := range messages {
		response, err := h.decodeMessage(message)
		if err != nil {
			writeError(c, http.StatusInternalServerError, "decrypt_failed", "could not decrypt synchronized message", nil)
			return
		}
		responses = append(responses, response)
	}
	c.JSON(http.StatusOK, gin.H{"messages": responses})
}

func (h *Handler) decodeMessage(message mls.Message) (messageResponse, error) {
	plaintext, err := h.codec.Decrypt(message.Epoch, message.Ciphertext)
	if err != nil {
		return messageResponse{}, err
	}
	return messageResponse{
		ID: message.ID, Epoch: message.Epoch, Sender: message.Sender,
		Message:          string(plaintext),
		EncryptedMessage: base64.StdEncoding.EncodeToString(message.Ciphertext),
		Timestamp:        message.Timestamp,
	}, nil
}

func writeClientError(c *gin.Context, err error) {
	if apiErr, ok := err.(*mls.APIError); ok {
		writeError(c, apiErr.StatusCode, apiErr.Code, apiErr.Message, apiErr.CurrentEpoch)
		return
	}
	writeError(c, http.StatusBadGateway, "backend_unavailable", "could not reach message service", nil)
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
