package Flash

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSetFlash(t *testing.T) {
	flashes := []Flash{
		{
			Level:   INFO,
			Message: "Test message",
		},
	}
	w := httptest.NewRecorder()
	err := SetFlashes(w, flashes)
	if err != nil {
		t.Errorf("Failed to set flash: %s", err)
	}
}

func TestGetFlash(t *testing.T) {
	flashes := []Flash{
		{
			Level:   INFO,
			Message: "Test message",
		},
	}
	w := httptest.NewRecorder()
	err := SetFlashes(w, flashes)
	if err != nil {
		t.Errorf("Failed to set flash: %s", err)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(w.Result().Cookies()[0])
	flashes, err = GetFlashes(w, r)
	if err != nil {
		t.Errorf("Failed to get flash: %s", err)
	}
	if len(flashes) == 0 {
		t.Errorf("No flashes found")
	}
}

func TestGetFlashNoCookie(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	flashes, err := GetFlashes(w, r)
	if err != nil {
		t.Errorf("Failed to get flash: %s", err)
	}
	if len(flashes) != 0 {
		t.Errorf("Flashes found")
	}
}

func TestGetFlashError(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: "_flash", Value: "invalid"})
	_, err := GetFlashes(w, r)
	if err == nil {
		t.Errorf("Failed to get flash")
	}
}

func TestGetClass(t *testing.T) {
	f := Flash{
		Level:   INFO,
		Message: "Test message",
	}
	if f.GetClass() != "alert-success" {
		t.Errorf("Invalid class")
	}

	f = Flash{
		Level:   WARN,
		Message: "Test message",
	}
	if f.GetClass() != "alert-warning" {
		t.Errorf("Invalid class")
	}

	f = Flash{
		Level:   ERROR,
		Message: "Test message",
	}
	if f.GetClass() != "alert-danger" {
		t.Errorf("Invalid class")
	}
}
