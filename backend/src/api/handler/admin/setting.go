package admin

import (
	"strings"

	"github.com/gin-gonic/gin"
	"opinion-analysis/pkg/response"
	"opinion-analysis/src/repository"
)

type SettingHandler struct {
	system *repository.SystemRepository
}

func NewSettingHandler(store *repository.Store) *SettingHandler {
	return &SettingHandler{system: store.System}
}

func (h *SettingHandler) List(c *gin.Context) {
	list, err := h.system.ListSettings()
	if err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, list)
}

type updateSettingReq struct {
	Value string `json:"value"`
}

var allowedSettingKeys = map[string]struct{}{
	"registration_enabled":          {},
	"dashboard.hot_topic_threshold": {},
}

func (h *SettingHandler) Update(c *gin.Context) {
	key := strings.TrimSpace(c.Param("key"))
	if _, ok := allowedSettingKeys[key]; !ok {
		response.Fail(c, 400, "unknown setting key")
		return
	}
	var req updateSettingReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	uid, _ := c.Get("userID")
	cu, _ := uid.(uint)

	out, err := h.system.UpsertSetting(key, req.Value, cu)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, out)
}
