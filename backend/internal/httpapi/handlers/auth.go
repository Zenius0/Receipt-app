package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/selim/fatura-backend/internal/auth"
	"github.com/selim/fatura-backend/internal/httpx"
	"github.com/selim/fatura-backend/internal/middleware"
	"github.com/selim/fatura-backend/internal/store"
)

const maxBodyBytes = 16 << 10

type AuthHandler struct {
	service *auth.Service
	logger  *slog.Logger
}

func NewAuthHandler(service *auth.Service, logger *slog.Logger) *AuthHandler {
	return &AuthHandler{service: service, logger: logger}
}

type signInRequest struct {
	Token string `json:"token"`
	Name  string `json:"name"`
	Nonce string `json:"nonce"`
}

func (h *AuthHandler) Google(w http.ResponseWriter, r *http.Request) {
	h.signIn(w, r, "google")
}

func (h *AuthHandler) Apple(w http.ResponseWriter, r *http.Request) {
	h.signIn(w, r, "apple")
}

func (h *AuthHandler) signIn(w http.ResponseWriter, r *http.Request, provider string) {
	var req signInRequest

	if err := decodeJSON(w, r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if strings.TrimSpace(req.Token) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_request", "token is required")
		return
	}

	ctx := r.Context()
	if provider == "apple" {
		ctx = auth.WithRawNonce(ctx, req.Nonce)
	}

	session, err := h.service.SignIn(ctx, provider, auth.SignInRequest{
		Token: req.Token,
		Name:  req.Name,
	})
	if err != nil {
		h.handleError(w, r, err)
		return
	}

	h.logger.InfoContext(ctx, "user signed in",
		slog.String("user_id", session.User.ID.String()),
		slog.String("provider", provider),
		slog.String("request_id", middleware.RequestIDFromContext(ctx)),
	)

	httpx.WriteJSON(w, http.StatusOK, session)
}

func (h *AuthHandler) handleError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrInvalidToken):
		httpx.WriteError(w, http.StatusUnauthorized, "invalid_token", "provider token could not be verified")
	case errors.Is(err, auth.ErrEmailNotVerified):
		httpx.WriteError(w, http.StatusUnauthorized, "email_not_verified", "provider email is not verified")
	case errors.Is(err, auth.ErrUnsupportedProvider):
		httpx.WriteError(w, http.StatusNotFound, "unsupported_provider", "provider is not supported")
	case errors.Is(err, store.ErrEmailConflict):
		httpx.WriteError(w, http.StatusConflict, "email_conflict", "this email is already registered with another sign-in method")
	default:
		h.logger.ErrorContext(r.Context(), "sign in failed",
			slog.String("error", err.Error()),
			slog.String("request_id", middleware.RequestIDFromContext(r.Context())),
		)
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "unexpected error")
	}
}

type UserHandler struct {
	service *auth.Service
}

func NewUserHandler(service *auth.Service) *UserHandler {
	return &UserHandler{service: service}
}

func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing session context")
		return
	}

	user, err := h.service.Authenticate(r.Context(), claims)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "session user no longer exists")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, "internal_error", "unexpected error")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, user)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	defer func() { _ = r.Body.Close() }()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return errors.New("request body too large")
		}
		if errors.Is(err, io.EOF) {
			return errors.New("request body is required")
		}
		return errors.New("request body must be valid json")
	}
	if decoder.More() {
		return errors.New("request body must contain a single json object")
	}

	return nil
}
