package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/momaek/tolato/server/internal/model"
	"github.com/momaek/tolato/server/internal/store"
)

// JWTSecret is set during initialization.
var JWTSecret string

// Gin context keys carrying the authenticated identity. Handlers read these
// through the CurrentUser* helpers rather than by string literal.
const (
	CtxUserID   = "user_id"
	CtxUsername = "username"
	CtxUserRole = "user_role"
)

// Claims defines the JWT claims structure.
type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateToken creates a new JWT token for a user.
func GenerateToken(u *model.User) (string, time.Time, error) {
	expiresAt := time.Now().Add(24 * time.Hour)
	claims := &Claims{
		UserID:   u.ID,
		Username: u.Username,
		Role:     u.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   u.ID,
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "tolato",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(JWTSecret))
	if err != nil {
		return "", time.Time{}, err
	}
	return tokenString, expiresAt, nil
}

// JWTAuth validates the bearer token and pins the caller's identity onto the
// request context.
//
// The role is re-read from the database on every request rather than trusted
// from the token, so demoting or disabling a user takes effect immediately
// instead of at token expiry (up to 24h later).
func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := parseBearerClaims(c)
		if !ok {
			return
		}

		// Tokens issued before multi-user support carry no user_id. Reject them
		// rather than guessing an identity; the user simply logs in again.
		if claims.UserID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "token predates user accounts, please sign in again",
			})
			return
		}

		user, err := store.GetUserByID(claims.UserID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "account no longer exists",
			})
			return
		}
		if user.Status != model.UserStatusActive {
			c.AbortWithStatusJSON(http.StatusForbidden, model.ErrorResponse{
				Error:   "forbidden",
				Message: "account is disabled",
			})
			return
		}

		c.Set(CtxUserID, user.ID)
		c.Set(CtxUsername, user.Username)
		c.Set(CtxUserRole, user.Role)
		c.Next()
	}
}

// RequireAdmin rejects non-admin callers. Mounted after JWTAuth.
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetString(CtxUserRole) != model.RoleAdmin {
			c.AbortWithStatusJSON(http.StatusForbidden, model.ErrorResponse{
				Error:   "forbidden",
				Message: "administrator privileges required",
			})
			return
		}
		c.Next()
	}
}

// parseBearerClaims extracts and verifies the JWT, aborting with 401 on any
// problem. The bool reports whether the caller should continue.
func parseBearerClaims(c *gin.Context) (*Claims, bool) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{
			Error:   "unauthorized",
			Message: "missing authorization header",
		})
		return nil, false
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{
			Error:   "unauthorized",
			Message: "invalid authorization header format",
		})
		return nil, false
	}

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(parts[1], claims, func(token *jwt.Token) (any, error) {
		return []byte(JWTSecret), nil
	})
	if err != nil || !token.Valid {
		c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{
			Error:   "unauthorized",
			Message: "invalid or expired token",
		})
		return nil, false
	}
	return claims, true
}

// ParseTokenString verifies a raw token and returns its claims. Used by the
// WebSocket handlers, which carry the token in a query parameter because
// browsers can't set headers on a WebSocket handshake.
func ParseTokenString(tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (any, error) {
		return []byte(JWTSecret), nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}
	return claims, nil
}

// CurrentUserID returns the authenticated user's ID.
func CurrentUserID(c *gin.Context) string { return c.GetString(CtxUserID) }

// CurrentUsername returns the authenticated user's username.
func CurrentUsername(c *gin.Context) string { return c.GetString(CtxUsername) }

// IsAdmin reports whether the caller holds the admin role.
func IsAdmin(c *gin.Context) bool { return c.GetString(CtxUserRole) == model.RoleAdmin }
