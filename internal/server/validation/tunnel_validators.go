package validation

import (
	"errors"
	"regexp"
)

var (
	ErrBannedName = errors.New("tunnel name contains invalid characters")
)

type TunnelValidator struct {
	nameRegex *regexp.Regexp
}

func NewTunnelValidator() *TunnelValidator {
	return &TunnelValidator{
		nameRegex: regexp.MustCompile(`^[a-z0-9-]{1,20}$`),
	}
}

func (v *TunnelValidator) ValidateTunnelRegister(name string) error {
	if !v.nameRegex.MatchString(name) {
		return ErrBannedName
	}
	return nil
}
