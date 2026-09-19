package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
)

const MaxAbsMove = 400

var (
	ErrInvalidFrame = errors.New("invalid frame")
	ErrNonfinite    = errors.New("nonfinite delta")
)

type Command interface{ command() }

type Auth struct{ Code string }
type Move struct{ Dx, Dy float64 }
type Click struct{ Button Button }
type Scroll struct{ Dx, Dy float64 }
type KeyPress struct{ Key Key }
type ChordPress struct{ Chord Chord }

func (Auth) command()        {}
func (Move) command()        {}
func (Click) command()       {}
func (Scroll) command()      {}
func (KeyPress) command()    {}
func (ChordPress) command()  {}

type Button string
type Key string
type Chord string

const (
	ButtonLeft  Button = "left"
	ButtonRight Button = "right"
	KeyLeft     Key    = "left"
	KeyRight    Key    = "right"
	KeyEsc      Key    = "esc"
	SpaceLeft   Chord  = "spaceLeft"
	SpaceRight  Chord  = "spaceRight"
	MissionCtrl Chord  = "missionControl"
)

type Reply interface{ reply() }

type Status struct{}
type Bye struct{ Reason string }

func (Status) reply() {}
func (Bye) reply()    {}

const (
	ReasonKicked    = "kicked"
	ReasonDisplaced = "displaced"
	ReasonBadAuth   = "badauth"
)

func ParseCommand(data []byte) (Command, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return nil, ErrInvalidFrame
	}
	t, _ := raw["t"].(string)
	switch t {
	case "auth":
		code, ok := raw["code"].(string)
		if !ok {
			return nil, ErrInvalidFrame
		}
		return Auth{Code: code}, nil
	case "move", "scroll":
		dx, err := number(raw["dx"])
		if err != nil {
			return nil, err
		}
		dy, err := number(raw["dy"])
		if err != nil {
			return nil, err
		}
		if t == "move" {
			return Move{Dx: dx, Dy: dy}, nil
		}
		return Scroll{Dx: dx, Dy: dy}, nil
	case "click":
		b, ok := raw["b"].(string)
		if !ok || (b != string(ButtonLeft) && b != string(ButtonRight)) {
			return nil, ErrInvalidFrame
		}
		return Click{Button: Button(b)}, nil
	case "key":
		k, ok := raw["k"].(string)
		if !ok || (k != string(KeyLeft) && k != string(KeyRight) && k != string(KeyEsc)) {
			return nil, ErrInvalidFrame
		}
		return KeyPress{Key: Key(k)}, nil
	case "chord":
		k, ok := raw["k"].(string)
		if !ok || (k != string(SpaceLeft) && k != string(SpaceRight) && k != string(MissionCtrl)) {
			return nil, ErrInvalidFrame
		}
		return ChordPress{Chord: Chord(k)}, nil
	default:
		return nil, ErrInvalidFrame
	}
}

func ParseReply(data []byte) (Reply, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return nil, ErrInvalidFrame
	}
	t, _ := raw["t"].(string)
	switch t {
	case "status":
		return Status{}, nil
	case "bye":
		reason, ok := raw["reason"].(string)
		if !ok {
			return nil, ErrInvalidFrame
		}
		return Bye{Reason: reason}, nil
	default:
		return nil, ErrInvalidFrame
	}
}

func EncodeCommand(command Command) ([]byte, error) {
	switch v := command.(type) {
	case Auth:
		return json.Marshal(struct {
			T    string `json:"t"`
			Code string `json:"code"`
		}{"auth", v.Code})
	case Move:
		if err := finite(v.Dx, v.Dy); err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			T  string  `json:"t"`
			Dx float64 `json:"dx"`
			Dy float64 `json:"dy"`
		}{"move", v.Dx, v.Dy})
	case Scroll:
		if err := finite(v.Dx, v.Dy); err != nil {
			return nil, err
		}
		return json.Marshal(struct {
			T  string  `json:"t"`
			Dx float64 `json:"dx"`
			Dy float64 `json:"dy"`
		}{"scroll", v.Dx, v.Dy})
	case Click:
		return json.Marshal(struct {
			T string `json:"t"`
			B string `json:"b"`
		}{"click", string(v.Button)})
	case KeyPress:
		return json.Marshal(struct {
			T string `json:"t"`
			K string `json:"k"`
		}{"key", string(v.Key)})
	case ChordPress:
		return json.Marshal(struct {
			T string `json:"t"`
			K string `json:"k"`
		}{"chord", string(v.Chord)})
	default:
		return nil, fmt.Errorf("unknown command")
	}
}

func EncodeReply(reply Reply) ([]byte, error) {
	switch v := reply.(type) {
	case Status:
		return json.Marshal(struct {
			T string `json:"t"`
		}{"status"})
	case Bye:
		return json.Marshal(struct {
			T      string `json:"t"`
			Reason string `json:"reason"`
		}{"bye", v.Reason})
	default:
		return nil, fmt.Errorf("unknown reply")
	}
}

func MoveInRange(dx, dy float64) bool {
	return math.Abs(dx) <= MaxAbsMove && math.Abs(dy) <= MaxAbsMove
}

func number(value any) (float64, error) {
	n, ok := value.(json.Number)
	if !ok {
		return 0, ErrInvalidFrame
	}
	text := strings.TrimSpace(n.String())
	if text == "NaN" || text == "Infinity" || text == "-Infinity" {
		return 0, ErrNonfinite
	}
	f, err := n.Float64()
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, ErrNonfinite
	}
	return f, nil
}

func finite(dx, dy float64) error {
	if math.IsInf(dx, 0) || math.IsNaN(dx) || math.IsInf(dy, 0) || math.IsNaN(dy) {
		return ErrNonfinite
	}
	return nil
}
