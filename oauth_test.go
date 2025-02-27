package main

import (
	"net/http/httptest"
	"testing"
)

func TestHasAuthorizationCode(t *testing.T) {
	values := map[string]string{
		authorizationCodeKey: "authCode",
		refreshTokenKey:      "refreshToken",
	}
	if !hasAuthorizationCode(values) {
		t.Errorf("Expected true, got false")
	}
}

func TestHasAuthorizationCodeNoAuthCode(t *testing.T) {
	values := map[string]string{
		refreshTokenKey: "refreshToken",
	}
	if hasAuthorizationCode(values) {
		t.Errorf("Expected false, got true")
	}
}

func TestHasAuthorizationCodeNoRefreshToken(t *testing.T) {
	values := map[string]string{
		authorizationCodeKey: "authCode",
	}
	if hasAuthorizationCode(values) {
		t.Errorf("Expected false, got true")
	}
}

func TestGetRedirectURL(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Host = "example.com"
	if getRedirectURL(r) != "http://example.com/code" {
		t.Errorf("Expected http://example.com/code, got %s", getRedirectURL(r))
	}
}
