package gatewayauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	TenantHeader    = "X-Kube-Cost-Tenant-ID"
	IdentityHeader  = "X-Kube-Cost-Gateway-Identity"
	TimestampHeader = "X-Kube-Cost-Gateway-Timestamp"
	SignatureHeader = "X-Kube-Cost-Gateway-Signature"
)

var (
	ErrMissingSignature = errors.New("gateway signature is required")
	ErrInvalidTimestamp = errors.New("gateway signature timestamp is invalid")
	ErrExpiredSignature = errors.New("gateway signature timestamp is outside allowed skew")
	ErrInvalidSignature = errors.New("gateway signature is invalid")
)

func SignRequest(r *http.Request, identity, key string, now time.Time) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	identity = strings.TrimSpace(identity)
	if identity == "" {
		identity = "gateway"
	}
	timestamp := strconv.FormatInt(now.UTC().Unix(), 10)
	r.Header.Set(IdentityHeader, identity)
	r.Header.Set(TimestampHeader, timestamp)
	r.Header.Set(SignatureHeader, signature(r, identity, timestamp, key))
}

func VerifyRequest(r *http.Request, key string, now time.Time, maxSkew time.Duration) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	identity := strings.TrimSpace(r.Header.Get(IdentityHeader))
	timestamp := strings.TrimSpace(r.Header.Get(TimestampHeader))
	actual := strings.TrimSpace(r.Header.Get(SignatureHeader))
	if identity == "" || timestamp == "" || actual == "" {
		return ErrMissingSignature
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return ErrInvalidTimestamp
	}
	signedAt := time.Unix(seconds, 0).UTC()
	if maxSkew > 0 {
		if now.UTC().Sub(signedAt) > maxSkew || signedAt.Sub(now.UTC()) > maxSkew {
			return ErrExpiredSignature
		}
	}
	expected := signature(r, identity, timestamp, key)
	if !hmac.Equal([]byte(actual), []byte(expected)) {
		return ErrInvalidSignature
	}
	return nil
}

func signature(r *http.Request, identity, timestamp, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(r.Method))
	_, _ = mac.Write([]byte{'\n'})
	_, _ = mac.Write([]byte(r.URL.EscapedPath()))
	_, _ = mac.Write([]byte{'\n'})
	_, _ = mac.Write([]byte(r.URL.RawQuery))
	_, _ = mac.Write([]byte{'\n'})
	_, _ = mac.Write([]byte(r.Header.Get(TenantHeader)))
	_, _ = mac.Write([]byte{'\n'})
	_, _ = mac.Write([]byte(identity))
	_, _ = mac.Write([]byte{'\n'})
	_, _ = mac.Write([]byte(timestamp))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
