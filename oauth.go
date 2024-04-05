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
	"github.com/gorilla/sessions"
)

func hasAuthorizationCode(session *sessions.Session) bool {
	_, authOk := session.Values["authorizationCode"]
	_, refreshOk := session.Values["refresh_token"]
	return authOk && refreshOk
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
	//authURL, _ := url.Parse("https://accounts.google.com/o/oauth2/v2/auth")
	//params := url.Values{}
	//params.Add("client_id", clientID)
	//params.Add("redirect_uri", redirectURL)
	//params.Add("response_type", "code")
	//params.Add("scope", "https://www.googleapis.com/auth/sdm.service")
	//params.Add("state", GetStateValueForSession(r))
	//params.Add("access_type", "offline")
	//params.Add("prompt", "consent")
	//authURL.RawQuery = params.Encode()
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

	session, err := cookieStore.Get(r, "session")
	if err != nil {
		log.Print(err.Error())
	}
	err = ts.ExecuteTemplate(w, "base", map[string]interface{}{"authorizeURL": authURL, "Flashes": session.Flashes()})
	if err != nil {
		log.Print(err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func code(w http.ResponseWriter, r *http.Request) {
	session, err := cookieStore.Get(r, "session")
	if err != nil {
		log.Print(err.Error())
	}
	//expectedState := GetStateValueForSession(r)
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

	//if state, ok := parsedQuery["state"]; ok {
	//	if state[0] != expectedState {
	//		log.Printf("State: %s does not match the expected value: %s", &state, expectedState)
	//		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	//		return
	//	}
	//} else {
	//	log.Printf("Didn't find the state in the URL")
	//	http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	//	return
	//}

	// TODO. Handle parsedQuery["error"] ...
	// https://developers.google.com/identity/protocols/oauth2/web-server#authorization-errors

	if code, ok := parsedQuery["code"]; ok {
		session.Values["authorizationCode"] = code[0]
		token, err := GetTokenFromAuthCode(code[0], getRedirectURL(r))
		if err != nil {
			http.Error(w, fmt.Sprintf("Error: %s", err), http.StatusInternalServerError)
			return
		}
		// Initial call required
		GetDevices(token.AccessToken)
		session.Values["refresh_token"] = token.RefreshToken
		// MaxAge == 1 year
		session.Options.MaxAge = 365 * 24 * 3600
		session.AddFlash(flash.Flash{
			Level:   flash.INFO,
			Message: "Successfully authenticated with Google",
		})
		err = session.Save(r, w)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error: %s", err), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
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
	log.Printf("Authenticate uri: %s, status code: %d\n", uri, resp.StatusCode)

	// TODO check response code

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
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
	return _authenticate(uri)
}

// TODO implement token exchange: https://developers.google.com/identity/protocols/oauth2/web-server#exchange-authorization-code

// View https://developers.google.com/nest/device-access/authorize
