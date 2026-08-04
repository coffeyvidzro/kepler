package event

import (
	"fmt"
	"strings"
)

type Version string

const CurrentVersion Version = "1"

func ParseVersion(value string) (Version, error) {
	version := Version(strings.TrimSpace(value))
	switch version {
	case CurrentVersion:
		return version, nil
	default:
		return "", fmt.Errorf("unsupported event version %q", value)
	}
}

func (version Version) Valid() bool {
	_, err := ParseVersion(string(version))
	return err == nil
}
