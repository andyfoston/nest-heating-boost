package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"

	flash "github.com/andyfoston/nest-heating-boost/flash"
)

const (
	authorizationCodeKey = "authorizationCode"
	refreshTokenKey      = "refresh_token"
)

//func hasAuthorizationCode(session *sessions.Session) bool {
//	authCode, authOk := session.Values[authorizationCodeKey]
//	refreshToken, refreshOk := session.Values[refreshTokenKey]
//	return authOk && refreshOk && authCode != "" && refreshToken != ""
//}

func hasAuthorizationCode(values map[string]string) bool {
	authCode, authOk := values[authorizationCodeKey]
	refreshToken, refreshOk := values[refreshTokenKey]
	return authOk && refreshOk && authCode != "" && refreshToken != ""
}

func getRedirectURL(r *http.Request) string {
	overrideRedirectURL := os.Getenv("OVERRIDE_REDIRECT_URL")
	if overrideRedirectURL != "" {
		return overrideRedirectURL
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/code", scheme, r.Host)
}

func AuthorizeAccess(w http.ResponseWriter, r *http.Request) {
	files := []string{
		"./templates/base.tmpl",
		"./templates/authorize.tmpl",
	}
	ts, err := template.ParseFiles(files...)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	redirectURL := getRedirectURL(r)
	authURL, _ := url.Parse(fmt.Sprintf(
		"https://nestservices.google.com/partnerconnections/%s/auth", projectID,
	))
	params := url.Values{
		"redirect_uri":  {redirectURL},
		"access_type":   {"offline"},
		"prompt":        {"consent"},
		"client_id":     {clientID},
		"response_type": {"code"},
		"scope":         {"https://www.googleapis.com/auth/sdm.service"},
	}
	authURL.RawQuery = params.Encode()

	flashes, err := flash.GetFlashes(w, r)
	if err != nil {
		log.Print(err.Error())
	}
	err = ts.ExecuteTemplate(w, "base", map[string]interface{}{"authorizeURL": authURL, "Flashes": flashes})
	if err != nil {
		log.Print(err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func code(w http.ResponseWriter, r *http.Request) {
	data, err := getCookie(r)
	if err != nil {
		log.Print(err.Error())
		data = make(map[string]string)
	}
	parsedURL, err := url.Parse(r.RequestURI)
	if err != nil {
		log.Printf("Failed to parse URL: %s", r.RequestURI)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	parsedQuery, err := url.ParseQuery(parsedURL.RawQuery)
	if err != nil {
		log.Printf("Failed to parse URL Query: %s", parsedURL.RawQuery)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// TODO. Handle parsedQuery["error"] ...
	// https://developers.google.com/identity/protocols/oauth2/web-server#authorization-errors

	if code, ok := parsedQuery["code"]; ok {
		data[authorizationCodeKey] = code[0]
		token, err := GetTokenFromAuthCode(code[0], getRedirectURL(r))
		if err != nil {
			http.Error(w, fmt.Sprintf("Error: %s", err), http.StatusInternalServerError)
			return
		}
		// Initial call required
		GetDevices(token.AccessToken)
		data[refreshTokenKey] = token.RefreshToken
		setCookie(data, w)
		flashes := make([]flash.Flash, 0, 1)
		flashes = append(flashes, flash.Flash{
			Level:   flash.INFO,
			Message: "Successfully authenticated with Google",
		})
		flash.SetFlashes(w, flashes)
		http.Redirect(w, r, "/", http.StatusSeeOther)
	} else if errorCode, ok := parsedQuery["error"]; ok {
		w.Write([]byte(fmt.Sprintf("Got an error response from Google: %s", errorCode[0])))
	} else {
		log.Printf("Missing code from URL: %s", parsedURL.RawQuery)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

type Token struct {
	AccessToken    string `json:"access_token"`
	ExpiresIn      int64  `json:"expires_in"`
	RefreshToken   string `json:"refresh_token"`
	tokenRequested time.Time
}

func (t *Token) TokenValid() bool {
	return t.tokenRequested.Add(time.Second * time.Duration(t.ExpiresIn)).Before(time.Now())
}

func _authenticate(uri string) (*Token, error) {
	req, err := http.NewRequest("POST", uri, bytes.NewReader([]byte{}))
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	log.Printf("Authenticate uri: %s, status code: %d\n", uri, resp.StatusCode)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("Unexpected response: %s", body)
	}

	token := Token{}
	err = json.Unmarshal(body, &token)
	if err != nil {
		return nil, err
	}
	token.tokenRequested = time.Now()
	return &token, nil
}

func GetTokenFromAuthCode(authCode string, redirectURI string) (*Token, error) {
	uri := fmt.Sprintf("https://www.googleapis.com/oauth2/v4/token?client_id=%s&client_secret=%s&code=%s&grant_type=authorization_code&redirect_uri=%s",
		clientID, clientSecret, authCode, redirectURI,
	)
	return _authenticate(uri)
}

func GetTokenFromRefreshToken(refreshToken string) (*Token, error) {
	uri := fmt.Sprintf("https://www.googleapis.com/oauth2/v4/token?client_id=%s&client_secret=%s&refresh_token=%s&grant_type=refresh_token",
		clientID, clientSecret, refreshToken,
	)
	token, err := _authenticate(uri)
	if err == nil {
		// re-inject refresh token into response
		token.RefreshToken = refreshToken

	}
	return token, err
}

// TODO implement token exchange: https://developers.google.com/identity/protocols/oauth2/web-server#exchange-authorization-code

// View https://developers.google.com/nest/device-access/authorize
