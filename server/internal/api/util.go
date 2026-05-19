package api

import "github.com/google/uuid"

func parseUUID(s string) (uuid.UUID, error) {
	if s == "" {
		return uuid.Nil, errEmptyUUID
	}
	return uuid.Parse(s)
}

type stringError string

func (e stringError) Error() string { return string(e) }

const errEmptyUUID stringError = "value is empty"
