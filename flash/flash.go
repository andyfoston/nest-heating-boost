package Flash

import (
	"bytes"
	"encoding/base64"
	"encoding/gob"
	"log"
	"net/http"
	"time"
)

type base int

const (
	INFO base = iota
	WARN
	ERROR
)

type Flash struct {
	Level   base
	Message string
}

func (f *Flash) GetClass() string {
	if f.Level == INFO {
		return "alert-success"
	} else if f.Level == WARN {
		return "alert-warning"
	} else {
		return "alert-danger"
	}
}

func SetFlashes(w http.ResponseWriter, flashes []Flash) error {
	var value bytes.Buffer
	enc := gob.NewEncoder(&value)

	err := enc.Encode(flashes)
	if err != nil {
		return err
	}
	v := base64.StdEncoding.EncodeToString(value.Bytes())
	log.Printf("Flash value: %s", v)
	cookie := &http.Cookie{Name: "_flash", Value: v, Path: "/"}
	http.SetCookie(w, cookie)
	return nil
}

func GetFlashes(w http.ResponseWriter, r *http.Request) ([]Flash, error) {
	c, err := r.Cookie("_flash")
	if err != nil {
		switch err {
		case http.ErrNoCookie:
			return []Flash{}, nil
		default:
			return nil, err
		}
	}

	dc := &http.Cookie{Name: "_flash", MaxAge: -1, Expires: time.Unix(1, 0), Path: "/"}
	http.SetCookie(w, dc)

	var value bytes.Buffer
	decoded, err := base64.StdEncoding.DecodeString(c.Value)
	if err != nil {
		return nil, err
	}
	value.WriteString(string(decoded))
	dec := gob.NewDecoder(&value)
	flashes := make([]Flash, 0, 5)
	err = dec.Decode(&flashes)
	if err != nil {
		return nil, err
	}
	return flashes, nil
}

func init() {
	gob.Register(&Flash{})
}
