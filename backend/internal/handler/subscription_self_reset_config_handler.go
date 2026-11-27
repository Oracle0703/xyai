package handler

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *SubscriptionSelfResetHandler) GetConfig(c *gin.Context) {
	config, err := h.service.Config(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, config)
}

func (h *SubscriptionSelfResetHandler) SaveConfig(c *gin.Context) {
	change, err := readSelfResetConfigChange(c.Request.Body)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	revision, err := h.service.SaveConfig(c.Request.Context(), change)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"revision": revision})
}

func readSelfResetConfigChange(reader io.Reader) (service.SubscriptionSelfResetConfigChange, error) {
	var change service.SubscriptionSelfResetConfigChange
	invalid := errors.BadRequest("INVALID_SELF_RESET_CONFIG", "Invalid self reset configuration change")
	const maxBody = 64 * 1024
	body, err := io.ReadAll(io.LimitReader(reader, maxBody+1))
	if err != nil || len(body) > maxBody {
		return change, invalid
	}
	var input struct {
		ExpectedRevision int64           `json:"expected_revision"`
		Policy           json.RawMessage `json:"policy"`
		Groups           json.RawMessage `json:"changes_by_organization"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || decoder.Decode(new(any)) != io.EOF || input.ExpectedRevision < 1 {
		return change, invalid
	}
	change.ExpectedRevision = input.ExpectedRevision
	change.Policy = input.Policy
	if input.Groups != nil {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(input.Groups, &raw); err != nil {
			return change, service.ErrInvalidSelfResetGroups
		}
		change.Groups, err = parseSelfResetGroupChanges(raw)
		if err != nil {
			return change, err
		}
	}
	return change, nil
}

func (h *SubscriptionSelfResetHandler) CloseConfig(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1))
	if err != nil || len(body) != 0 {
		response.BadRequest(c, "Emergency close does not accept configuration fields")
		return
	}
	revision, err := h.service.CloseConfig(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"revision": revision})
}
