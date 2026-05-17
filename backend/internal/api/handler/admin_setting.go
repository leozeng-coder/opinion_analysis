package handler

import (
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"opinion-analysis/internal/model"
	"opinion-analysis/pkg/response"
)

type AdminSettingHandler struct {
	db *gorm.DB
}

func NewAdminSettingHandler(db *gorm.DB) *AdminSettingHandler {
	return &AdminSettingHandler{db: db}
}

func (h *AdminSettingHandler) List(c *gin.Context) {
	var list []model.SystemSetting
	if err := h.db.Order("`key`").Find(&list).Error; err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, list)
}

type updateSettingReq struct {
	Value string `json:"value"`
}

var allowedSettingKeys = map[string]struct{}{
	"registration_enabled": {},
}

func (h *AdminSettingHandler) Update(c *gin.Context) {
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

	var existing model.SystemSetting
	err := h.db.Where("`key` = ?", key).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		if err := h.db.Create(&model.SystemSetting{Key: key, Value: req.Value, UpdatedBy: cu}).Error; err != nil {
			response.ServerError(c)
			return
		}
	} else if err != nil {
		response.ServerError(c)
		return
	} else {
		if err := h.db.Model(&model.SystemSetting{}).Where("`key` = ?", key).
			Updates(map[string]interface{}{"value": req.Value, "updated_by": cu}).Error; err != nil {
			response.ServerError(c)
			return
		}
	}
	var out model.SystemSetting
	h.db.Where("`key` = ?", key).First(&out)
	response.OK(c, out)
}

// GetRegistrationEnabled 暴露给 auth.go 用，判断是否允许注册。
func GetRegistrationEnabled(db *gorm.DB) bool {
	var s model.SystemSetting
	if err := db.Where("`key` = ?", "registration_enabled").First(&s).Error; err != nil {
		// 默认允许（兼容尚未 seed 的环境）
		return true
	}
	v := strings.ToLower(strings.TrimSpace(s.Value))
	return v == "true" || v == "1" || v == "yes" || v == "on"
}
