package e2ee

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	store *Store
}

type proposalRequest struct {
	Epoch uint64 `json:"epoch" binding:"required"`
	Data  string `json:"data" binding:"required"`
}

func NewRouter(store *Store) http.Handler {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery(), cors())
	h := &Handler{store: store}
	router.GET("/e2ee/v1/global-room/sync", h.sync)
	router.POST("/e2ee/v1/global-room/proposal", h.proposal)
	return router
}

func (h *Handler) sync(c *gin.Context) {
	rawEpoch := c.Query("local_epoch")
	if rawEpoch == "" {
		rawEpoch = c.Query("local-epoch")
	}
	if rawEpoch == "" {
		writeError(c, http.StatusBadRequest, "invalid_request", "local_epoch is required", nil)
		return
	}
	localEpoch, err := strconv.ParseUint(rawEpoch, 10, 64)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "local_epoch must be a non-negative integer", nil)
		return
	}

	currentEpoch, commits, err := h.store.CommitsAfter(localEpoch)
	if errors.Is(err, ErrEpochMismatch) {
		writeError(c, http.StatusConflict, "epoch_mismatch", "local epoch is newer than the server epoch", &currentEpoch)
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "could not read E2EE state", nil)
		return
	}
	if commits == nil {
		commits = []Commit{}
	}
	c.JSON(http.StatusOK, gin.H{"current_epoch": currentEpoch, "commits": commits})
}

func (h *Handler) proposal(c *gin.Context) {
	var request proposalRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "epoch and base64 data are required", nil)
		return
	}
	if _, err := base64.StdEncoding.DecodeString(request.Data); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "data must be valid base64", nil)
		return
	}

	err := h.store.AddProposal(Commit{Epoch: request.Epoch, Data: request.Data})
	if errors.Is(err, ErrEpochMismatch) {
		currentEpoch := h.store.CurrentEpoch()
		writeError(c, http.StatusConflict, "epoch_mismatch", "proposal epoch must equal current_epoch + 1", &currentEpoch)
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "internal_error", "could not persist proposal", nil)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"current_epoch": request.Epoch})
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
