package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"opinion-analysis/config"
	"opinion-analysis/internal/model"
	"opinion-analysis/pkg/response"
	"opinion-analysis/pkg/utils"
	"gorm.io/gorm"
)

type AuthHandler struct {
	db *gorm.DB
}

func NewAuthHandler(db *gorm.DB) *AuthHandler {
	return &AuthHandler{db: db}
}

type loginReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type registerReq struct {
	Username string `json:"username" binding:"required,min=3,max=32"`
	Password string `json:"password" binding:"required,min=6"`
	Email    string `json:"email" binding:"required,email"`
	Nickname string `json:"nickname"`
}

// Login godoc
// @Summary 用户登录
// @Tags auth
// @Accept json
// @Produce json
// @Param body body loginReq true "登录信息"
// @Success 200 {object} response.Response
// @Router /api/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	var user model.User
	if err := h.db.Where("username = ?", req.Username).First(&user).Error; err != nil {
		response.Fail(c, 1001, "用户名或密码错误")
		return
	}
	if user.Status == 0 {
		response.Fail(c, 1002, "账号已被禁用")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		response.Fail(c, 1001, "用户名或密码错误")
		return
	}

	token, err := utils.GenerateToken(user.ID, user.Username, user.Role,
		config.Cfg.JWT.Secret, config.Cfg.JWT.ExpireHour)
	if err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, gin.H{"token": token, "user": user})
}

// Register godoc
// @Summary 用户注册
// @Tags auth
// @Accept json
// @Produce json
// @Param body body registerReq true "注册信息"
// @Success 200 {object} response.Response
// @Router /api/auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	var count int64
	h.db.Model(&model.User{}).Where("username = ? OR email = ?", req.Username, req.Email).Count(&count)
	if count > 0 {
		response.Fail(c, 1003, "用户名或邮箱已存在")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		response.ServerError(c)
		return
	}

	user := model.User{
		Username: req.Username,
		Password: string(hash),
		Email:    req.Email,
		Nickname: req.Nickname,
		Role:     "viewer",
	}
	if err := h.db.Create(&user).Error; err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, gin.H{"id": user.ID})
}

func (h *AuthHandler) Profile(c *gin.Context) {
	userID, _ := c.Get("userID")
	var user model.User
	if err := h.db.First(&user, userID).Error; err != nil {
		response.Fail(c, 404, "用户不存在")
		return
	}
	response.OK(c, user)
}
