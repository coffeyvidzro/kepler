package messagetemplate

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"

	"github.com/labstack/echo/v5"

	module "github.com/coffeyvidzro/dugble/server/internal/modules/messagetemplate"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) Create(c *echo.Context) error { var req module.CreateRequest;if err:=decodeJSON(c,&req,false);err!=nil{return err};value,err:=h.service.Create(c.Request().Context(),req);if err!=nil{return httputil.Error(c,err)};return httputil.Created(c,value) }
func (h *Handler) List(c *echo.Context) error { values,err:=h.service.List(c.Request().Context(),module.ListRequest{Limit:parseInt32(c.QueryParam("limit")),Offset:parseInt32(c.QueryParam("offset"))});if err!=nil{return httputil.Error(c,err)};return httputil.OK(c,values) }
func (h *Handler) Get(c *echo.Context) error { value,err:=h.service.Get(c.Request().Context(),c.Param("template"));if err!=nil{return httputil.Error(c,err)};return httputil.OK(c,value) }
func (h *Handler) Update(c *echo.Context) error { var req module.UpdateRequest;if err:=decodeJSON(c,&req,false);err!=nil{return err};value,err:=h.service.Update(c.Request().Context(),c.Param("template"),req);if err!=nil{return httputil.Error(c,err)};return httputil.OK(c,value) }
func (h *Handler) Delete(c *echo.Context) error { value,err:=h.service.Delete(c.Request().Context(),c.Param("template"));if err!=nil{return httputil.Error(c,err)};return httputil.OK(c,value) }
func (h *Handler) Publish(c *echo.Context) error { var req module.PublishRequest;if err:=decodeJSON(c,&req,true);err!=nil{return err};value,err:=h.service.Publish(c.Request().Context(),c.Param("template"),req);if err!=nil{return httputil.Error(c,err)};return httputil.OK(c,value) }
func (h *Handler) Duplicate(c *echo.Context) error { var req module.DuplicateRequest;if err:=decodeJSON(c,&req,false);err!=nil{return err};value,err:=h.service.Duplicate(c.Request().Context(),c.Param("template"),req);if err!=nil{return httputil.Error(c,err)};return httputil.Created(c,value) }
func (h *Handler) ListVersions(c *echo.Context) error { values,err:=h.service.ListVersions(c.Request().Context(),c.Param("template"),module.ListRequest{Limit:parseInt32(c.QueryParam("limit")),Offset:parseInt32(c.QueryParam("offset"))});if err!=nil{return httputil.Error(c,err)};return httputil.OK(c,values) }
func (h *Handler) GetVersion(c *echo.Context) error { value,err:=h.service.GetVersion(c.Request().Context(),c.Param("template"),c.Param("version_id"));if err!=nil{return httputil.Error(c,err)};return httputil.OK(c,value) }
func (h *Handler) Revert(c *echo.Context) error { value,err:=h.service.Revert(c.Request().Context(),c.Param("template"),c.Param("version_id"));if err!=nil{return httputil.Error(c,err)};return httputil.OK(c,value) }
func (h *Handler) Preview(c *echo.Context) error { var req module.PreviewRequest;if err:=decodeJSON(c,&req,true);err!=nil{return err};value,err:=h.service.Preview(c.Request().Context(),c.Param("template"),req);if err!=nil{return httputil.Error(c,err)};return httputil.OK(c,value) }
func (h *Handler) TestSend(c *echo.Context) error { var req module.TestSendRequest;if err:=decodeJSON(c,&req,false);err!=nil{return err};value,err:=h.service.TestSend(c.Request().Context(),c.Param("template"),req);if err!=nil{return httputil.Error(c,err)};return httputil.Accepted(c,value) }

func decodeJSON(c *echo.Context,dst any,optional bool) error { decoder:=json.NewDecoder(c.Request().Body);decoder.UseNumber();if err:=decoder.Decode(dst);err!=nil{if optional&&errors.Is(err,io.EOF){return nil};return httputil.Error(c,apperrors.NewBadRequest("Invalid JSON request body"))};return nil }
func parseInt32(value string)int32{parsed,err:=strconv.ParseInt(value,10,32);if err!=nil{return 0};return int32(parsed)}
