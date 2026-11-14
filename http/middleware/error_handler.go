package middleware

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"multi-tenant-hybrid-search/apperror"
	"multi-tenant-hybrid-search/constants"
	"multi-tenant-hybrid-search/http/response"
)

// ErrorHandler is the one place where errors become HTTP responses. Controllers and middleware only call
// c.Error(err); after the handlers ran, this writes {"error": "<message>"} with the right status:
//
//	*AppError                      its Code, Message and Header
//	*ValidationError               400 with its message
//	ErrNotFound                    404 "not found"
//	ErrEmbeddingUnavailable        503 "embedding service unavailable"
//	anything else                  500 "internal error" (the cause is never sent to the client)
//
// 5xx responses are logged at ERROR with method, path and cause; 4xx are not logged.
func ErrorHandler(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 || c.Writer.Written() {
			return
		}
		err := c.Errors.Last().Err
		status, message, header := classify(err)
		if status >= http.StatusInternalServerError {
			log.Error("request failed", "method", c.Request.Method, "path", c.FullPath(), "status", status, "error", err)
		}
		for k, v := range header {
			c.Header(k, v)
		}
		c.JSON(status, response.ErrorResponse{Error: message})
	}
}

func classify(err error) (int, string, map[string]string) {
	var appErr *apperror.AppError
	var valErr *apperror.ValidationError
	switch {
	case errors.As(err, &appErr):
		return appErr.Code, appErr.Message, appErr.Header
	case errors.As(err, &valErr):
		return http.StatusBadRequest, valErr.Message, nil
	case errors.Is(err, apperror.ErrNotFound):
		return http.StatusNotFound, constants.MsgNotFound, nil
	case errors.Is(err, apperror.ErrEmbeddingUnavailable):
		return http.StatusServiceUnavailable, constants.MsgEmbeddingUnavailable, nil
	default:
		return http.StatusInternalServerError, constants.MsgInternalError, nil
	}
}
