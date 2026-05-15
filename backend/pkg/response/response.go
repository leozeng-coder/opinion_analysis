package response

import (
	"net/http"
	"github.com/gin-gonic/gin"
)

type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type PageData struct {
	Total int64 `json:"total"`
	List  any   `json:"list"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Response{Code: 0, Message: "success", Data: data})
}

func OKPage(c *gin.Context, total int64, list any) {
	c.JSON(http.StatusOK, Response{Code: 0, Message: "success", Data: PageData{Total: total, List: list}})
}

func Fail(c *gin.Context, code int, msg string) {
	c.JSON(http.StatusOK, Response{Code: code, Message: msg})
}

func Unauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, Response{Code: 401, Message: "unauthorized"})
}

func Forbidden(c *gin.Context) {
	c.JSON(http.StatusForbidden, Response{Code: 403, Message: "forbidden"})
}

func ServerError(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: "internal server error"})
}
