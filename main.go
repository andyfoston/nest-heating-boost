package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	Flash "github.com/andyfoston/nest-heating-boost/flash"
	_ "github.com/gorilla/sessions"
	redistore "gopkg.in/boj/redistore.v1"
)

var (
	cookieStore  = getCookieStore()
	projectID    = os.Getenv("PROJECT_ID")
	clientID     = os.Getenv("CLIENT_ID")
	clientSecret = os.Getenv("CLIENT_SECRET")
)

func getCookieStore() *redistore.RediStore {
	store, err := redistore.NewRediStore(10, "tcp", os.Getenv("REDIS_ENDPOINT"), os.Getenv("REDIS_PASSWORD"), []byte(os.Getenv("SESSION_KEY")))
	if err != nil {
		panic(err)
	}
	return store
}

// Get temperature from device

func homePage(w http.ResponseWriter, r *http.Request) {
	session, err := cookieStore.Get(r, "session")
	if err != nil {
		log.Print(err.Error())
	}
	if !hasAuthorizationCode(session) {
		err = session.Save(r, w)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error: %s", err), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, "/authorize", http.StatusTemporaryRedirect)
		return
	}

	files := []string{
		"./templates/base.tmpl",
		"./templates/home.tmpl",
	}
	ts, err := template.ParseFiles(files...)
	if err != nil {
		log.Print(err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// FIXME handle errors
	refreshToken := session.Values["refresh_token"].(string)
	token, _ := GetTokenFromRefreshToken(refreshToken)
	//fmt.Printf("New Token: %s\n", token.AccessToken)
	devices, _ := GetDevices(token.AccessToken)
	fmt.Printf("DEVICES: %v\n", devices)
	if devices == nil {
		log.Println("Unable to get a list of devices")
		session.AddFlash(Flash.Flash{
			Level:   Flash.WARN,
			Message: "Unable to get a list of devices. Please try authorization access to Nest again",
		})
		session.Save(r, w)
		http.Redirect(w, r, "/authorize", http.StatusTemporaryRedirect)
		return
	}
	err = ts.ExecuteTemplate(w, "base", map[string]interface{}{"Flashes": session.Flashes(), "Devices": devices.Devices})
	if err != nil {
		log.Println(err.Error())
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	session.Save(r, w)
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
	} else if newToken.AccessToken != "" && token.AccessToken != newToken.AccessToken {
		token = *newToken
	}
	newTemperature, err := GetTemperature(token.AccessToken, deviceId)
	if err != nil {
		log.Printf("Failed to get temperature before resetting to normal: %s", err)
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
		session, err := cookieStore.Get(r, "session")
		if err != nil {
			log.Print(err.Error())
		}

		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		deviceId := r.FormValue("device")
		temperature, tempErr := strconv.ParseFloat(r.FormValue("temperature"), 32)
		duration, durationErr := strconv.ParseInt(r.FormValue("duration"), 10, 16)
		if deviceId == "" {
			session.AddFlash(Flash.Flash{
				Level:   Flash.ERROR,
				Message: "Unable to find the thermostat to adjust",
			})
		}
		if tempErr != nil {
			session.AddFlash(Flash.Flash{
				Level:   Flash.ERROR,
				Message: fmt.Sprintf("Unable to get the temperature to set to: %s", tempErr),
			})
		}
		if durationErr != nil {
			session.AddFlash(Flash.Flash{
				Level:   Flash.ERROR,
				Message: fmt.Sprintf("Unable to get the duration to run the heating for: %s", tempErr),
			})
		}
		if deviceId == "" || tempErr != nil || durationErr != nil {
			session.Save(r, w)
			return
		}

		// FIXME handle error
		refreshToken := session.Values["refresh_token"].(string)
		token, _ := GetTokenFromRefreshToken(refreshToken)
		go runBoost(*token, deviceId, float32(temperature), int(duration))
		session.AddFlash(Flash.Flash{
			Level:   Flash.INFO,
			Message: fmt.Sprintf("Setting heating to %.1f °C for %d minute(s)", temperature, duration),
		})
		session.Save(r, w)
	})
	mux.HandleFunc("/", homePage)
	mux.HandleFunc("/authorize", AuthorizeAccess)
	mux.HandleFunc("/code", code)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		_, err := cookieStore.Get(r, "session")
		if err != nil {
			http.Error(w, fmt.Sprintf("Unable to access cookiestore: %s", err.Error()), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	http.ListenAndServe(":8080", mux)
}
