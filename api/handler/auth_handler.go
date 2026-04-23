package handler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"unicode"

	"github.com/bigduu/bodhi-server/api/dto"
	"github.com/bigduu/bodhi-server/internal/auth"
	"github.com/bigduu/bodhi-server/internal/config"
	"github.com/bigduu/bodhi-server/internal/models"
	"github.com/bigduu/bodhi-server/internal/security"
)

type AuthHandler struct {
	db    *sql.DB
	cfg   *config.Config
	guard *security.LoginGuard
}

func NewAuthHandler(db *sql.DB, cfg *config.Config, guard *security.LoginGuard) *AuthHandler {
	return &AuthHandler{db: db, cfg: cfg, guard: guard}
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req dto.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.Username) < 3 {
		writeError(w, http.StatusBadRequest, "username must be >= 3 chars")
		return
	}
	if err := validatePassword(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Check registration settings
	if !models.IsRegistrationEnabled(r.Context(), h.db) {
		writeError(w, http.StatusForbidden, "registration is disabled")
		return
	}
	if models.IsInviteRequired(r.Context(), h.db) {
		if req.InviteCode == "" {
			writeError(w, http.StatusBadRequest, "invite code required")
			return
		}
		if err := models.ValidateInviteCode(r.Context(), h.db, req.InviteCode); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	user, err := models.CreateUser(r.Context(), h.db, req.Username, req.Password, req.Email)
	if err != nil {
		if err == models.ErrUserExists {
			writeError(w, http.StatusConflict, "username already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	// Consume invite code if used
	if req.InviteCode != "" {
		models.UseInviteCode(r.Context(), h.db, req.InviteCode)
	}

	tokenResp, err := h.generateTokenPair(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate tokens")
		return
	}

	writeJSON(w, http.StatusCreated, tokenResp)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if !h.guard.Check(r) {
		writeError(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	}

	var req dto.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	user, err := models.AuthenticateUser(r.Context(), h.db, req.Username, req.Password)
	if err != nil {
		h.guard.RecordFail(r)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	h.guard.Reset(r)

	tokenResp, err := h.generateTokenPair(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate tokens")
		return
	}

	writeJSON(w, http.StatusOK, tokenResp)
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req dto.RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	claims, err := auth.ValidateToken(req.RefreshToken, h.cfg.Auth.JWTSecret, auth.RefreshToken)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	// Check if this specific token was revoked
	revoked, _ := auth.IsTokenRevoked(r.Context(), h.db, claims.ID)
	if revoked {
		writeError(w, http.StatusUnauthorized, "token has been revoked")
		return
	}
	// Check if all user tokens were revoked (e.g. password change)
	if claims.IssuedAt != nil {
		userRevoked, _ := auth.IsUserTokensRevoked(r.Context(), h.db, claims.Subject, claims.IssuedAt.Time)
		if userRevoked {
			writeError(w, http.StatusUnauthorized, "token has been revoked")
			return
		}
	}

	user, err := models.GetUserByID(r.Context(), h.db, claims.Subject)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "user not found")
		return
	}

	// Revoke old refresh token to prevent reuse
	if claims.ID != "" && claims.ExpiresAt != nil {
		auth.RevokeToken(r.Context(), h.db, claims.ID, claims.ExpiresAt.Time)
	}

	tokenResp, err := h.generateTokenPair(user)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate tokens")
		return
	}

	writeJSON(w, http.StatusOK, tokenResp)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := r.Header.Get("X-User-ID")
	user, err := models.GetUserByID(r.Context(), h.db, userID)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	writeJSON(w, http.StatusOK, dto.UserResponse{
		ID:        user.ID,
		Username:  user.Username,
		Email:     user.Email.String,
		IsAdmin:   user.IsAdmin,
		CreatedAt: user.CreatedAt,
	})
}

func (h *AuthHandler) generateTokenPair(user *models.User) (*dto.AuthResponse, error) {
	accessToken, err := auth.GenerateAccessToken(user.ID, user.Username, h.cfg.Auth.JWTSecret, h.cfg.Auth.AccessTokenTTL)
	if err != nil {
		return nil, err
	}

	refreshToken, err := auth.GenerateRefreshToken(user.ID, user.Username, h.cfg.Auth.JWTSecret, h.cfg.Auth.RefreshTokenTTL)
	if err != nil {
		return nil, err
	}

	return &dto.AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(h.cfg.Auth.AccessTokenTTL.Seconds()),
		User: dto.UserResponse{
			ID:        user.ID,
			Username:  user.Username,
			IsAdmin:   user.IsAdmin,
			CreatedAt: user.CreatedAt,
		},
	}, nil
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(dto.ErrorResponse{Error: msg})
}

func writeInternalError(w http.ResponseWriter, err error, context string) {
	slog.Error(context, "error", err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(dto.ErrorResponse{Error: "internal error"})
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return errors.New("password must be at least 8 characters")
	}
	var hasUpper, hasLower, hasDigit bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}
	if !hasUpper {
		return errors.New("password must contain at least one uppercase letter")
	}
	if !hasLower {
		return errors.New("password must contain at least one lowercase letter")
	}
	if !hasDigit {
		return errors.New("password must contain at least one digit")
	}
	return nil
}
