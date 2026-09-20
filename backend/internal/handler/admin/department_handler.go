package admin

import (
	"encoding/json"
	"io"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type DepartmentHandler struct{ departments *service.DepartmentService }

func NewDepartmentHandler(departments *service.DepartmentService) *DepartmentHandler {
	return &DepartmentHandler{departments: departments}
}

func departmentJSON(c *gin.Context, target any) bool {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		response.BadRequest(c, "invalid department request")
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		response.BadRequest(c, "request must contain a single JSON object")
		return false
	}
	return true
}

func departmentPathID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "invalid id")
		return 0, false
	}
	return id, true
}

func departmentListFilter(c *gin.Context) (service.DepartmentListFilter, bool) {
	f := service.DepartmentListFilter{Organization: c.Query("organization"), Status: c.Query("status"), Q: c.Query("q"), DepartmentID: c.Query("department_id")}
	if c.Request.URL.Query().Has("user_ids") {
		parts := strings.Split(c.Query("user_ids"), ",")
		if len(parts) > 200 {
			response.BadRequest(c, "too many users")
			return f, false
		}
		for _, part := range parts {
			id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err != nil || id <= 0 {
				response.BadRequest(c, "invalid user_ids")
				return f, false
			}
			f.UserIDs = append(f.UserIDs, id)
		}
	}
	for _, entry := range []struct {
		name  string
		value *int
	}{{"page", &f.Page}, {"page_size", &f.PageSize}} {
		if raw, ok := c.GetQuery(entry.name); ok {
			n, err := strconv.Atoi(raw)
			if err != nil || n <= 0 {
				response.BadRequest(c, "invalid "+entry.name)
				return f, false
			}
			*entry.value = n
		}
	}
	return f, true
}

func departmentReply(c *gin.Context, value any, err error) {
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, value)
}

func (h *DepartmentHandler) List(c *gin.Context) {
	f, ok := departmentListFilter(c)
	if !ok {
		return
	}
	v, err := h.departments.List(c.Request.Context(), f)
	departmentReply(c, v, err)
}

func (h *DepartmentHandler) Create(c *gin.Context) { h.save(c, 0) }
func (h *DepartmentHandler) Update(c *gin.Context) {
	id, ok := departmentPathID(c)
	if ok {
		h.save(c, id)
	}
}
func (h *DepartmentHandler) save(c *gin.Context, id int64) {
	var input *service.DepartmentSaveInput
	if !departmentJSON(c, &input) {
		return
	}
	if input == nil {
		response.BadRequest(c, "department object required")
		return
	}
	v, err := h.departments.Save(c.Request.Context(), id, *input)
	departmentReply(c, v, err)
}

func (h *DepartmentHandler) Members(c *gin.Context) {
	f, ok := departmentListFilter(c)
	if !ok {
		return
	}
	if c.Param("id") != "" {
		id, ok := departmentPathID(c)
		if !ok {
			return
		}
		f.DepartmentID = strconv.FormatInt(id, 10)
	}
	v, err := h.departments.Members(c.Request.Context(), f)
	departmentReply(c, v, err)
}

func (h *DepartmentHandler) Assign(c *gin.Context) {
	var input *struct {
		DepartmentID json.RawMessage                  `json:"department_id"`
		Members      []service.DepartmentMemberChange `json:"members"`
	}
	if !departmentJSON(c, &input) {
		return
	}
	if input == nil || len(input.DepartmentID) == 0 {
		response.BadRequest(c, "assignment object required")
		return
	}
	var departmentID *int64
	if err := json.Unmarshal(input.DepartmentID, &departmentID); err != nil {
		response.BadRequest(c, "invalid department id")
		return
	}
	count, err := h.departments.Assign(c.Request.Context(), service.DepartmentAssignInput{DepartmentID: departmentID, Members: input.Members})
	departmentReply(c, gin.H{"updated_count": count}, err)
}

func (h *DepartmentHandler) GetAccess(c *gin.Context) {
	id, ok := departmentPathID(c)
	if !ok {
		return
	}
	v, err := h.departments.GetAccess(c.Request.Context(), id)
	departmentReply(c, v, err)
}

func (h *DepartmentHandler) SetAccess(c *gin.Context) {
	id, ok := departmentPathID(c)
	if !ok {
		return
	}
	var input *service.DepartmentAccessInput
	if !departmentJSON(c, &input) {
		return
	}
	if input == nil {
		response.BadRequest(c, "scope object required")
		return
	}
	v, err := h.departments.SetAccess(c.Request.Context(), id, *input)
	departmentReply(c, v, err)
}

func (h *DepartmentHandler) ReportScope(c *gin.Context) {
	v, err := h.departments.Scope(c.Request.Context(), service.AdminPermissionOrganizationUsage)
	departmentReply(c, v, err)
}

func (h *DepartmentHandler) SubscriptionScope(c *gin.Context) {
	v, err := h.departments.Scope(c.Request.Context(), service.AdminPermissionDepartmentSubscriptions)
	departmentReply(c, v, err)
}

func (h *DepartmentHandler) SubscriptionUsers(c *gin.Context) {
	scope, err := h.departments.Scope(c.Request.Context(), service.AdminPermissionDepartmentSubscriptions)
	if err != nil {
		departmentReply(c, nil, err)
		return
	}
	org, dept, err := service.NormalizeDepartmentFilter(c.Query("organization"), c.Query("department_id"))
	if err == nil {
		err = scope.ValidateSelection(org, dept, "")
	}
	if err != nil {
		departmentReply(c, nil, err)
		return
	}
	result := []gin.H{}
	for _, m := range scope.SelectedMembers(org, dept, c.Query("q")) {
		result = append(result, gin.H{"id": m.ID, "email": m.Email, "deleted": false})
		if len(result) == 30 {
			break
		}
	}
	response.Success(c, result)
}

func (h *DepartmentHandler) SubscriptionGroups(c *gin.Context) {
	v, err := h.departments.SubscriptionGroups(c.Request.Context(), c.Query("q"))
	departmentReply(c, v, err)
}
