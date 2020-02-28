package duration

import (
	"errors"
	"strconv"
)

var unitMap = map[string]uint64{
	"ms":  1,
	"s":   1000,
	"m":   60000,
	"min": 60000,
	"h":   3600000,
	"d":   86400000,
	"w":   604800000,
}

// Parse parses duration string and returns its numeric representation
func Parse(s string, defaultUnit string) (duration uint64, err error) {
	orig := s
	neg := false
	if len(s) == 0 {
		return 0, nil
	}
	if s[0] == '-' {
		neg = true
		s = s[1:]
	}
	i := 0
	for ; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			break
		}
		if duration > (1<<63-1)/10 {
			return 0, errors.New("Overflow while parsing duration: " + orig)
		}
		duration = duration*10 + uint64(s[i]) - '0'
		if duration < 0 {
			return 0, errors.New("Overflow while parsing duration: " + orig)
		}
	}
	multiplier, ok := unitMap[s[i:]]
	if !ok {
		if i == len(s) {
			multiplier = unitMap[defaultUnit]
		} else {
			return 0, errors.New("Unknown unit " + (s[i:]) + " in duration " + orig + " " + strconv.Itoa(i))
		}
	}
	if duration > (1<<63-1)/multiplier {
		return 0, errors.New("Overflow while parsing duration: " + orig)
	}
	duration *= multiplier

	if neg {
		duration = -duration
	}
	return duration, nil
}
