package Flash

import "encoding/gob"

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

func init() {
	gob.Register(&Flash{})
}
