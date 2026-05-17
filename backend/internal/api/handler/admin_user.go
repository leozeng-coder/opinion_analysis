package handler

import (
	"crypto/rand"
	"math/big"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"opinion-analysis/internal/model"
	"opinion-analysis/pkg/response"
)

type AdminUserHandler struct {
	db *gorm.DB
}

func NewAdminUserHandler(db *gorm.DB) *AdminUserHandler {
	return &AdminUserHandler{db: db}
}

var allowedRoles = map[string]struct{}{
	"admin": {}, "analyst": {}, "viewer": {},
}

func (h *AdminUserHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}
	keyword := strings.TrimSpace(c.Query("keyword"))
	role := strings.TrimSpace(c.Query("role"))

	q := h.db.Model(&model.User{})
	if keyword != "" {
		like := "%" + keyword + "%"
		q = q.Where("username LIKE ? OR email LIKE ? OR nickname LIKE ?", like, like, like)
	}
	if role != "" {
		q = q.Where("role = ?", role)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		response.ServerError(c)
		return
	}
	var list []model.User
	if err := q.Order("id desc").Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		response.ServerError(c)
		return
	}
	response.OKPage(c, total, list)
}

type updateUserReq struct {
	Role     *string `json:"role"`
	Status   *int8   `json:"status"`
	Nickname *string `json:"nickname"`
	Email    *string `json:"email"`
}

func (h *AdminUserHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.Fail(c, 400, "invalid id")
		return
	}
	var req updateUserReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, 400, err.Error())
		return
	}
	updates := map[string]interface{}{}
	if req.Role != nil {
		if _, ok := allowedRoles[*req.Role]; !ok {
			response.Fail(c, 400, "role must be admin|analyst|viewer")
			return
		}
		updates["role"] = *req.Role
	}
	if req.Status != nil {
		if *req.Status != 0 && *req.Status != 1 {
			response.Fail(c, 400, "status must be 0 or 1")
			return
		}
		// 禁止把自己禁用 / 降权
		curUID, _ := c.Get("userID")
		if cu, ok := curUID.(uint); ok && cu == uint(id) && *req.Status == 0 {
			response.Fail(c, 1005, "不能禁用当前登录账号")
			return
		}
		updates["status"] = *req.Status
	}
	if req.Nickname != nil {
		updates["nickname"] = *req.Nickname
	}
	if req.Email != nil {
		updates["email"] = *req.Email
	}
	if len(updates) == 0 {
		response.Fail(c, 400, "nothing to update")
		return
	}
	if err := h.db.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		response.ServerError(c)
		return
	}
	var user model.User
	h.db.First(&user, id)
	response.OK(c, user)
}

func (h *AdminUserHandler) ResetPassword(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.Fail(c, 400, "invalid id")
		return
	}
	var user model.User
	if err := h.db.First(&user, id).Error; err != nil {
		response.Fail(c, 404, "用户不存在")
		return
	}
	plain := randAlphaNum(10)
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		response.ServerError(c)
		return
	}
	if err := h.db.Model(&model.User{}).Where("id = ?", id).Update("password", string(hash)).Error; err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, gin.H{"password": plain})
}

func (h *AdminUserHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		response.Fail(c, 400, "invalid id")
		return
	}
	curUID, _ := c.Get("userID")
	if cu, ok := curUID.(uint); ok && cu == uint(id) {
		response.Fail(c, 1005, "不能删除当前登录账号")
		return
	}
	if err := h.db.Delete(&model.User{}, id).Error; err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, nil)
}

func randAlphaNum(n int) string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789"
	b := make([]byte, n)
	max := big.NewInt(int64(len(alphabet)))
	for i := range b {
		idx, err := rand.Int(rand.Reader, max)
		if err != nil {
			b[i] = alphabet[0]
			continue
		}
		b[i] = alphabet[idx.Int64()]
	}
	return string(b)
}
