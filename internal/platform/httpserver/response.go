package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/cng1985/edugo/internal/shared/apperr"
	"github.com/cng1985/edugo/internal/shared/trace"
	"github.com/gin-gonic/gin"
)

type envelope struct {
	Code         int    `json:"code"`
	Msg          string `json:"msg,omitempty"`
	ErrorCode    string `json:"errorCode,omitempty"`
	Data         any    `json:"data"`
	Page         int    `json:"page,omitempty"`
	Size         int    `json:"size,omitempty"`
	Total        int64  `json:"total,omitempty"`
	ResponseType string `json:"responseType,omitempty"`
	TraceID      string `json:"traceId"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, envelope{Code: 200, Msg: "success", Data: data, TraceID: trace.From(c.Request.Context())})
}

func Page(c *gin.Context, data any, page, size int, total int64) {
	if data == nil {
		data = []any{}
	}
	c.JSON(http.StatusOK, envelope{
		Code: 200, Msg: "success", Data: data, Page: page, Size: size, Total: total,
		ResponseType: "page", TraceID: trace.From(c.Request.Context()),
	})
}

func One(c *gin.Context, data any) {
	c.JSON(http.StatusOK, envelope{Code: 200, Msg: "success", Data: data, TraceID: trace.From(c.Request.Context())})
}

func Fail(c *gin.Context, err error) {
	e := apperr.As(err)
	if e.Status == 500 && !errors.As(err, &e) {
		e = apperr.Internal("内部错误")
	}
	c.JSON(e.Status, envelope{
		Code: e.Status, ErrorCode: e.ErrorCode, Msg: e.Msg, Data: nil, TraceID: trace.From(c.Request.Context()),
	})
}

func Bind(c *gin.Context, dest any) error {
	dec := json.NewDecoder(c.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return apperr.Invalid("请求 JSON 无法解析")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return apperr.Invalid("请求 JSON 无法解析")
	}
	return nil
}
