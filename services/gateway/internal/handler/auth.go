package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/edysupardi/parkirpintar/pkg/auth"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

type AuthHandler struct {
	db        *pgxpool.Pool
	validator *auth.Validator
	expiryH   int
}

func NewAuth(db *pgxpool.Pool, validator *auth.Validator, expiryHours int) *AuthHandler {
	return &AuthHandler{db: db, validator: validator, expiryH: expiryHours}
}

type registerRequest struct {
	Name        string `json:"name"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	Password    string `json:"password"`
	VehicleType string `json:"vehicle_type"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type authResponse struct {
	DriverID    string `json:"driver_id"`
	Name        string `json:"name"`
	Email       string `json:"email"`
	Token       string `json:"token"`
	VehicleType string `json:"vehicle_type"`
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Email == "" || req.Password == "" || req.Phone == "" {
		writeError(w, http.StatusBadRequest, "name, email, phone, and password are required")
		return
	}
	if req.VehicleType == "" {
		req.VehicleType = "car"
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to hash password")
		return
	}

	var driverID string
	err = h.db.QueryRow(context.Background(),
		`INSERT INTO drivers (name, email, phone, password_hash, vehicle_type)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id`,
		req.Name, req.Email, req.Phone, string(hash), req.VehicleType,
	).Scan(&driverID)
	if err != nil {
		if isDuplicateEmail(err) {
			writeError(w, http.StatusConflict, "email already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create driver")
		return
	}

	token, err := h.validator.GenerateToken(driverID, req.VehicleType, h.expiryH)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	writeJSON(w, http.StatusCreated, authResponse{
		DriverID:    driverID,
		Name:        req.Name,
		Email:       req.Email,
		Token:       token,
		VehicleType: req.VehicleType,
	})
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	var driverID, name, passwordHash, vehicleType string
	err := h.db.QueryRow(context.Background(),
		`SELECT id, name, password_hash, vehicle_type FROM drivers WHERE email = $1`,
		req.Email,
	).Scan(&driverID, &name, &passwordHash, &vehicleType)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	token, err := h.validator.GenerateToken(driverID, vehicleType, h.expiryH)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	writeJSON(w, http.StatusOK, authResponse{
		DriverID:    driverID,
		Name:        name,
		Email:       req.Email,
		Token:       token,
		VehicleType: vehicleType,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func isDuplicateEmail(err error) bool {
	return err != nil && (contains(err.Error(), "duplicate key") || contains(err.Error(), "unique constraint"))
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
