package protocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"unicode/utf8"
)

const MaxAbsMove = 400

// MaxFrameBytes is the maximum attribute value carried by one BLE write.
const MaxFrameBytes = 512

// MinimumCommandBytes accommodates canonical finite-double movement frames.
const MinimumCommandBytes = 80

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

func (Auth) command()       {}
func (Move) command()       {}
func (Click) command()      {}
func (Scroll) command()     {}
func (KeyPress) command()   {}
func (ChordPress) command() {}

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
	raw, err := parseObject(data)
	if err != nil {
		return nil, err
	}
	t, _ := raw["t"].(string)
	fields := map[string][]string{
		"auth": {"t", "code"}, "move": {"t", "dx", "dy"}, "scroll": {"t", "dx", "dy"},
		"click": {"t", "b"}, "key": {"t", "k"}, "chord": {"t", "k"},
	}
	if !hasFields(raw, fields[t]) {
		return nil, ErrInvalidFrame
	}
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
	raw, err := parseObject(data)
	if err != nil {
		return nil, err
	}
	t, _ := raw["t"].(string)
	fields := map[string][]string{"status": {"t"}, "bye": {"t", "reason"}}
	if !hasFields(raw, fields[t]) {
		return nil, ErrInvalidFrame
	}
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
		if !utf8.ValidString(v.Code) {
			return nil, ErrInvalidFrame
		}
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
		if v.Button != ButtonLeft && v.Button != ButtonRight {
			return nil, ErrInvalidFrame
		}
		return json.Marshal(struct {
			T string `json:"t"`
			B string `json:"b"`
		}{"click", string(v.Button)})
	case KeyPress:
		if v.Key != KeyLeft && v.Key != KeyRight && v.Key != KeyEsc {
			return nil, ErrInvalidFrame
		}
		return json.Marshal(struct {
			T string `json:"t"`
			K string `json:"k"`
		}{"key", string(v.Key)})
	case ChordPress:
		if v.Chord != SpaceLeft && v.Chord != SpaceRight && v.Chord != MissionCtrl {
			return nil, ErrInvalidFrame
		}
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
		if !utf8.ValidString(v.Reason) {
			return nil, ErrInvalidFrame
		}
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

func parseObject(data []byte) (map[string]any, error) {
	if !utf8.Valid(data) || !validUnicodeEscapes(data) {
		return nil, ErrInvalidFrame
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	opening, err := dec.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, ErrInvalidFrame
	}
	raw := make(map[string]any)
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return nil, ErrInvalidFrame
		}
		key, ok := token.(string)
		if _, duplicate := raw[key]; !ok || duplicate {
			return nil, ErrInvalidFrame
		}
		var value any
		if err := dec.Decode(&value); err != nil {
			return nil, ErrInvalidFrame
		}
		raw[key] = value
	}
	if closing, err := dec.Token(); err != nil || closing != json.Delim('}') {
		return nil, ErrInvalidFrame
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, ErrInvalidFrame
	}
	return raw, nil
}

func hasFields(raw map[string]any, fields []string) bool {
	if len(fields) == 0 || len(raw) != len(fields) {
		return false
	}
	for _, field := range fields {
		if _, ok := raw[field]; !ok {
			return false
		}
	}
	return true
}

// encoding/json replaces isolated UTF-16 surrogates; Swift rejects them.
func validUnicodeEscapes(data []byte) bool {
	quoted := false
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			quoted = !quoted
			continue
		}
		if !quoted || data[i] != '\\' {
			continue
		}
		i++
		if i >= len(data) {
			return false
		}
		if data[i] != 'u' {
			continue
		}
		value, ok := unicodeEscape(data[i:])
		if !ok {
			return false
		}
		i += 4
		if value >= 0xdc00 && value <= 0xdfff {
			return false
		}
		if value >= 0xd800 && value <= 0xdbff {
			if i+2 >= len(data) || data[i+1] != '\\' {
				return false
			}
			low, ok := unicodeEscape(data[i+2:])
			if !ok || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}

func unicodeEscape(data []byte) (uint16, bool) {
	if len(data) < 5 || data[0] != 'u' {
		return 0, false
	}
	var value uint16
	for _, digit := range data[1:5] {
		value <<= 4
		switch {
		case digit >= '0' && digit <= '9':
			value |= uint16(digit - '0')
		case digit >= 'a' && digit <= 'f':
			value |= uint16(digit - 'a' + 10)
		case digit >= 'A' && digit <= 'F':
			value |= uint16(digit - 'A' + 10)
		default:
			return 0, false
		}
	}
	return value, true
}
