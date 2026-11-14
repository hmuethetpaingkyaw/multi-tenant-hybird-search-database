// Package controller holds the HTTP controllers. A controller parses the request, calls one use case and
// writes the success response; on failure it calls c.Error(err) and returns (see middleware.ErrorHandler).
package controller

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"multi-tenant-hybrid-search/apperror"
	"multi-tenant-hybrid-search/constants"
	"multi-tenant-hybrid-search/domain"
	"multi-tenant-hybrid-search/http/middleware"
)

// fail hands the error to the central error handler.
func fail(c *gin.Context, err error) {
	_ = c.Error(err)
}

func identityOf(c *gin.Context) (domain.Identity, error) {
	id, ok := middleware.IdentityFrom(c)
	if !ok {
		return domain.Identity{}, apperror.Unauthorized()
	}
	return id, nil
}

// documentIDParam returns the canonical form of the :id path parameter.
func documentIDParam(c *gin.Context) (string, error) {
	u, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return "", apperror.BadRequest(constants.MsgInvalidDocumentID, err)
	}
	return u.String(), nil
}

// bindStrictJSON decodes exactly one JSON object into dst: unknown fields are rejected and the size is capped.
func bindStrictJSON(c *gin.Context, dst any, maxBytes int64) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	if err == nil {
		// Exactly one JSON value: reject trailing data.
		if err2 := dec.Decode(&struct{}{}); err2 != io.EOF {
			err = errors.New("trailing data")
			if err2 != nil {
				err = err2
			}
		}
	}
	if err == nil {
		return nil
	}
	var tooBig *http.MaxBytesError
	switch {
	case errors.As(err, &tooBig):
		return apperror.PayloadTooLarge(err)
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		return apperror.BadRequest(constants.MsgInvalidBody+": "+strings.TrimPrefix(err.Error(), "json: "), err)
	default:
		return apperror.BadRequest(constants.MsgInvalidBody, err)
	}
}
