package admin

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	sessionCookie   = "seedstrem_session"
	sessionLifetime = 7 * 24 * time.Hour
)

// sessionKey derives the HMAC key from the admin password so rotating
// the password invalidates all sessions without server-side state.
func sessionKey(adminPassword string) []byte {
	sum := sha256.Sum256([]byte("seedstrem-session:" + adminPassword))
	return sum[:]
}

// mintSession returns a signed session value valid until expiry.
func mintSession(adminPassword string, expiry time.Time) string {
	payload := strconv.FormatInt(expiry.Unix(), 10)
	mac := hmac.New(sha256.New, sessionKey(adminPassword))
	mac.Write([]byte(payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return payload + "." + sig
}

// validSession checks a session value against the current password.
func validSession(adminPassword, value string, now time.Time) bool {
	payload, sig, ok := strings.Cut(value, ".")
	if !ok {
		return false
	}
	expiry, err := strconv.ParseInt(payload, 10, 64)
	if err != nil || now.Unix() >= expiry {
		return false
	}
	mac := hmac.New(sha256.New, sessionKey(adminPassword))
	mac.Write([]byte(payload))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return subtle.ConstantTimeCompare([]byte(sig), []byte(want)) == 1
}

func setSessionCookie(w http.ResponseWriter, value string, expiry time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		Expires:  expiry,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

// requireSession is middleware enforcing a valid session cookie plus a
// CSRF header on state-changing methods.
func (h *Handler) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil || !validSession(h.config.Get().Server.AdminPassword, cookie.Value, time.Now()) {
			writeJSONError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
				writeJSONError(w, http.StatusForbidden, "missing CSRF header")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	fmt.Fprintf(w, `{"error":%q}`, msg)
}
