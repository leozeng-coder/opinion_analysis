package user

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"opinion-analysis/config"
	"opinion-analysis/pkg/response"
	"opinion-analysis/pkg/utils"
	"opinion-analysis/src/model"
	"opinion-analysis/src/repository"
)

type AuthHandler struct {
	users  *repository.UserRepository
	system *repository.SystemRepository
}

func NewAuthHandler(store *repository.Store) *AuthHandler {
	return &AuthHandler{users: store.User, system: store.System}
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

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	user, err := h.users.FindByUsername(req.Username)
	if err != nil {
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

func (h *AuthHandler) Register(c *gin.Context) {
	if !h.system.RegistrationEnabled() {
		response.Fail(c, 1004, "开放注册已关闭，请联系管理员")
		return
	}
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, http.StatusBadRequest, err.Error())
		return
	}

	exists, err := h.users.ExistsByUsernameOrEmail(req.Username, req.Email)
	if err != nil {
		response.ServerError(c)
		return
	}
	if exists {
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
	if err := h.users.Create(&user); err != nil {
		response.ServerError(c)
		return
	}
	response.OK(c, gin.H{"id": user.ID})
}

func (h *AuthHandler) Profile(c *gin.Context) {
	userID, _ := c.Get("userID")
	user, err := h.users.FindByID(userID.(uint))
	if err != nil {
		response.Fail(c, 404, "用户不存在")
		return
	}
	response.OK(c, user)
}
