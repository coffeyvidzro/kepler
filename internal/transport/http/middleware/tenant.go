package middleware

import (
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v5"

	"github.com/coffeyvidzro/dugble/server/internal/platform/authnz"
	"github.com/coffeyvidzro/dugble/server/internal/platform/tenant"
	apperrors "github.com/coffeyvidzro/dugble/server/pkg/errors"
	"github.com/coffeyvidzro/dugble/server/pkg/httputil"
)

const defaultTenantParam = "team_id"
const defaultTenantHeader = "X-Team-ID"

type TenantConfig struct { Memberships tenant.MembershipStore; ParamName string; HeaderName string; Required tenant.Permission }
func Tenant(config TenantConfig) echo.MiddlewareFunc {
	paramName:=config.ParamName;if paramName==""{paramName=defaultTenantParam};headerName:=config.HeaderName;if headerName==""{headerName=defaultTenantHeader}
	return func(next echo.HandlerFunc) echo.HandlerFunc{return func(c *echo.Context) error{
		if config.Memberships==nil{return httputil.Error(c,apperrors.NewInternal("Tenant membership store is not configured",nil))}
		principal,ok:=authnz.PrincipalFromContext(c.Request().Context());if !ok{return httputil.Error(c,apperrors.NewUnauthorized("Authentication is required"))}
		teamID,err:=teamIDFromRequest(c,paramName,headerName);if err!=nil{return httputil.Error(c,err)}
		membership,err:=config.Memberships.GetTenantMembership(c.Request().Context(),teamID,principal.UserID);if err!=nil||!membership.Active(){return httputil.Error(c,apperrors.NewForbidden("Active team membership is required"))}
		access:=tenant.AccessContext{Actor:tenant.Actor{Type:tenant.ActorTypeUser,UserID:membership.UserID,SessionID:principal.SessionID},Scope:tenant.Scope{TeamID:membership.TeamID,Role:membership.Role,Status:membership.Status}}
		if decision:=tenant.Authorize(access,config.Required);!decision.Allowed{return httputil.Error(c,apperrors.NewForbidden(decision.Reason))}
		c.SetRequest(c.Request().WithContext(tenant.ContextWithAccess(c.Request().Context(),access)));return next(c)
	}}
}

type TenantAccessConfig struct { Sessions SessionStore; Users PrincipalRepository; Memberships tenant.MembershipStore; Tokens TeamTokenStore; CSRF CSRFConfig; TenantParam string; TenantHeader string; Required tenant.Permission }
func TenantAccess(config TenantAccessConfig) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc{return func(c *echo.Context) error{
		authorization:=strings.TrimSpace(c.Request().Header.Get(echo.HeaderAuthorization))
		if authorization==""{return SessionAuth(SessionAuthConfig{Sessions:config.Sessions,Users:config.Users})(CSRF(config.CSRF)(Tenant(TenantConfig{Memberships:config.Memberships,ParamName:config.TenantParam,HeaderName:config.TenantHeader,Required:config.Required})(next)))(c)}
		if _,ok:=parseBearerToken(authorization);!ok{return httputil.Error(c,apperrors.NewUnauthorized("Authorization header is invalid"))}
		return TeamToken(TeamTokenConfig{Tokens:config.Tokens,Required:config.Required})(next)(c)
	}}
}

func teamIDFromRequest(c *echo.Context,paramName,headerName string)(uuid.UUID,error){pathValue:=strings.TrimSpace(c.Param(paramName));headerValue:=strings.TrimSpace(c.Request().Header.Get(headerName));if pathValue==""&&headerValue==""{return uuid.Nil,apperrors.NewBadRequest("Team id is required")};var pathID uuid.UUID;if pathValue!=""{parsed,err:=uuid.Parse(pathValue);if err!=nil{return uuid.Nil,apperrors.NewBadRequest("Team id must be a valid UUID")};pathID=parsed};var headerID uuid.UUID;if headerValue!=""{parsed,err:=uuid.Parse(headerValue);if err!=nil{return uuid.Nil,apperrors.NewBadRequest("Team id must be a valid UUID")};headerID=parsed};if pathValue!=""&&headerValue!=""&&pathID!=headerID{return uuid.Nil,apperrors.NewBadRequest("Team id in path and header must match")};if pathValue!=""{return pathID,nil};return headerID,nil}
