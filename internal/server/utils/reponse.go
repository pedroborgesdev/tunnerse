package utils

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	CodeSuccess         = "success"
	CodeBadRequest      = "bad_request"
	CodeUnauthorized    = "unauthorized"
	CodeForbidden       = "forbidden"
	CodeNotFound        = "not_found"
	CodeConflict        = "conflict"
	CodeTooManyRequests = "too_many_requests"
	CodeInternalError   = "internal_error"
)

const (
	MsgSuccess            = "Operation successful"
	MsgBadRequest         = "Invalid request data"
	MsgUnauthorized       = "Unauthorized access"
	MsgForbidden          = "Access forbidden"
	MsgNotFound           = "Resource not found"
	MsgConflict           = "Resource already exists"
	MsgTooManyRequests    = "Too many requests"
	MsgInternalError      = "Internal server error"
	MsgValidationError    = "Validation failed"
	MsgInvalidCredentials = "Invalid credentials"
)

type APIResponse struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Status  int         `json:"status"`
}

var codeToStatus = map[string]int{
	CodeSuccess:         http.StatusOK,
	CodeBadRequest:      http.StatusBadRequest,
	CodeUnauthorized:    http.StatusUnauthorized,
	CodeForbidden:       http.StatusForbidden,
	CodeNotFound:        http.StatusNotFound,
	CodeConflict:        http.StatusConflict,
	CodeTooManyRequests: http.StatusTooManyRequests,
	CodeInternalError:   http.StatusInternalServerError,
}

func NewResponse(code, message string, data interface{}) APIResponse {
	return APIResponse{
		Code:    code,
		Message: message,
		Data:    data,
		Status:  codeToStatus[code],
	}
}

func Send(c *gin.Context, code, message string, data interface{}) {
	c.JSON(codeToStatus[code], NewResponse(code, message, data))
}

func AbortWith(c *gin.Context, code, message string, data interface{}) {
	c.AbortWithStatusJSON(codeToStatus[code], NewResponse(code, message, data))
}

func Success(c *gin.Context, data interface{}) {
	Send(c, CodeSuccess, MsgSuccess, data)
}

func BadRequest(c *gin.Context, data interface{}) {
	AbortWith(c, CodeBadRequest, MsgBadRequest, data)
}

func Unauthorized(c *gin.Context, data interface{}) {
	AbortWith(c, CodeUnauthorized, MsgUnauthorized, data)
}

func Forbidden(c *gin.Context, data interface{}) {
	AbortWith(c, CodeForbidden, MsgForbidden, data)
}

func NotFound(c *gin.Context, data interface{}) {
	AbortWith(c, CodeNotFound, MsgNotFound, data)
}

func Conflict(c *gin.Context, data interface{}) {
	AbortWith(c, CodeConflict, MsgConflict, data)
}

func TooManyRequests(c *gin.Context, data interface{}) {
	AbortWith(c, CodeTooManyRequests, MsgTooManyRequests, data)
}

func InternalError(c *gin.Context, data interface{}) {
	AbortWith(c, CodeInternalError, MsgInternalError, data)
}
