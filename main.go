package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	flash "github.com/andyfoston/nest-heating-boost/flash"
	"github.com/gorilla/securecookie"
)

var (
	projectID    = os.Getenv("PROJECT_ID")
	clientID     = os.Getenv("CLIENT_ID")
	clientSecret = os.Getenv("CLIENT_SECRET")
	hashKey      = os.Getenv("HASH_KEY")
	blockKey     = os.Getenv("BLOCK_KEY")
	s            = securecookie.New([]byte(hashKey), []byte(blockKey))
	httpsCookie  = strings.ToLower(os.Getenv("INSECURE_COOKIE")) != "true"
)

const (
	COOKIE_NAME      = "nest-boost"
	THERMOSTAT_CACHE = "thermostat-cache"
)

func _setCookie(value map[string]string, w http.ResponseWriter, cookieName string, maxAge int) error {
	encoded, err := s.Encode(cookieName, value)
	if err == nil {
		cookie := &http.Cookie{
			Name:     cookieName,
			Value:    encoded,
			Path:     "/",
			Secure:   httpsCookie,
			HttpOnly: true,
			MaxAge:   maxAge,
		}
		http.SetCookie(w, cookie)
	}
	return err
}

func setCookie(value map[string]string, w http.ResponseWriter) error {
	return _setCookie(value, w, COOKIE_NAME, 0)
}

func setCacheCookie(value map[string]string, w http.ResponseWriter) error {
	return _setCookie(value, w, "_cache", 600)
}

func _getCookie(r *http.Request, cookieName string) (map[string]string, error) {
	cookie, err := r.Cookie(cookieName)
	value := make(map[string]string)
	if err != nil {
		switch err {
		case http.ErrNoCookie:
			return value, nil
		default:
			return nil, err
		}
	}
	err = s.Decode(cookieName, cookie.Value, &value)
	if err != nil {
		return nil, err
	}
	return value, nil
}

func getCookie(r *http.Request) (map[string]string, error) {
	return _getCookie(r, COOKIE_NAME)
}

func getCacheCookie(r *http.Request) (map[string]string, error) {
	return _getCookie(r, "_cache")
}

// Get temperature from device

func homePage(w http.ResponseWriter, r *http.Request) {
	data, err := getCookie(r)
	if err != nil {
		log.Printf("Unable to get cookie: %s\n", err.Error())
	}
	if !hasAuthorizationCode(data) {
		log.Println("No authorization code found. Redirecting to /authorize")
		http.Redirect(w, r, "/authorize", http.StatusSeeOther)
		return
	}

	files := []string{
		"./templates/base.tmpl",
		"./templates/home.tmpl",
	}
	ts, err := template.ParseFiles(files...)
	if err != nil {
		log.Printf("Unable to parse files: %s\n", err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// FIXME handle errors
	flashes := make([]flash.Flash, 0)
	refreshToken := data[refreshTokenKey]
	token, err := GetTokenFromRefreshToken(refreshToken)
	if err != nil {
		log.Printf("Failed to get token from refresh token: %s\n", err)
		http.Redirect(w, r, "/authorize", http.StatusSeeOther)
		return
	}
	enableSubmit := true
	devices, err := GetDevices(token.AccessToken)
	if err != nil {
		switch err {
		case RateLimitError:
			flashes = append(flashes, flash.Flash{
				Level:   flash.WARN,
				Message: "Google have blocked requests temporarily. Please try again in a couple of minutes.",
			})
			enableSubmit = false
		default:
			log.Println("Unable to get a list of devices. Token: ", token.AccessToken, "Error:", err)
			flashes = append(flashes, flash.Flash{
				Level:   flash.WARN,
				Message: "Unable to get a list of devices. Please try authorization access to Nest again",
			})
			flash.SetFlashes(w, flashes)
			http.Redirect(w, r, "/authorize", http.StatusSeeOther)
			return
		}
	} else {
		f, err := flash.GetFlashes(w, r)
		if err != nil {
			log.Printf("Failed to get flashes: %s\n", err)
		} else {
			flashes = f
		}
	}
	err = ts.ExecuteTemplate(w, "base", map[string]interface{}{"Flashes": flashes, "Devices": devices.Devices, "enableSubmit": enableSubmit})
	if err != nil {
		log.Printf("Failed to execute template: %s\n", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
}

func runBoost(token Token, deviceId string, desiredTemp float32, duration int) {
	originalTemperature, err := GetTemperature(token.AccessToken, deviceId)
	if err != nil {
		log.Printf("Failed to get initial temperature: %s", err)
		return
	}

	err = SetTemperature(token.AccessToken, deviceId, desiredTemp)
	if err != nil {
		log.Printf("Failed to set temperature: %s", err)
		return
	}
	time.Sleep(time.Minute * time.Duration(duration))
	newToken, err := GetTokenFromRefreshToken(token.RefreshToken)
	if err != nil {
		log.Printf("Failed to get a new access token: %s", err)
		return
	}
	token = *newToken
	log.Printf("Token: %v", token)
	newTemperature, err := GetTemperature(token.AccessToken, deviceId)
	if err != nil {
		log.Println("Failed to get temperature before resetting to normal:", err)
		return
	}
	if *newTemperature != desiredTemp {
		log.Printf("Temperature has changed since boosting. Leaving as is: Current: %f, expected: %f", *newTemperature, desiredTemp)
		return
	}
	SetTemperature(token.AccessToken, deviceId, *originalTemperature)

}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/boost", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		// Setting the redirect before cookies appears to cause cookies not to be set
		defer http.Redirect(w, r, "/", http.StatusSeeOther)
		deviceId := r.FormValue("device")
		temperature, tempErr := strconv.ParseFloat(r.FormValue("temperature"), 32)
		duration, durationErr := strconv.ParseInt(r.FormValue("duration"), 10, 16)
		flashes := make([]flash.Flash, 0, 1)
		if deviceId == "" {
			flashes = append(flashes, flash.Flash{
				Level:   flash.ERROR,
				Message: "Unable to find the thermostat to adjust",
			})
		}
		if tempErr != nil {
			flashes = append(flashes, flash.Flash{
				Level:   flash.ERROR,
				Message: fmt.Sprintf("Unable to get the temperature to set to: %s", tempErr),
			})
		}
		if durationErr != nil {
			flashes = append(flashes, flash.Flash{
				Level:   flash.ERROR,
				Message: fmt.Sprintf("Unable to get the duration to run the heating for: %s", tempErr),
			})
		}
		if deviceId == "" || tempErr != nil || durationErr != nil {
			flash.SetFlashes(w, flashes)
			return
		}

		// FIXME handle error
		data, err := getCookie(r)
		if err != nil {
			log.Println("Failed to get cookie. Error", err.Error())
		}
		refreshToken := data[refreshTokenKey]
		log.Printf("refreshToken: %s\n", refreshToken)
		token, err := GetTokenFromRefreshToken(refreshToken)
		if err != nil {
			log.Printf("Failed: %s\n", err)
			flashes = append(flashes, flash.Flash{
				Level:   flash.ERROR,
				Message: fmt.Sprintf("Failed to run boost: %s", err),
			})
			flash.SetFlashes(w, flashes)
			return
		}
		log.Printf("Token: %v\n", token)
		go runBoost(*token, deviceId, float32(temperature), int(duration))
		flashes = append(flashes, flash.Flash{
			Level:   flash.INFO,
			Message: fmt.Sprintf("Setting heating to %s°C for %d minute(s)", r.FormValue("temperature"), duration),
		})
		err = flash.SetFlashes(w, flashes)
		if err != nil {
			log.Printf("Failed to set flash: %s\n", err)
		}
	})
	mux.HandleFunc("/", homePage)
	mux.HandleFunc("/authorize", AuthorizeAccess)
	mux.HandleFunc("/code", code)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	log.Print("Test")
	http.ListenAndServe(":8080", mux)
}
