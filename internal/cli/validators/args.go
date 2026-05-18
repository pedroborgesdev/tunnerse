package validators

import (
	"errors"
	"regexp"
)

var (
	ErrInvalidTunnelID = errors.New("tunnel id contains invalid characters")
	ErrInvalidAddress  = errors.New("address contains invalid characters or format")
)

type ArgsValidator struct {
	regexTunnelID *regexp.Regexp
	regexAddress  *regexp.Regexp
}

func NewArgsValidator() *ArgsValidator {
	return &ArgsValidator{
		regexTunnelID: regexp.MustCompile(`^[a-z0-9-]{1,20}$`),
		regexAddress:  regexp.MustCompile(`^((6553[0-5])|(655[0-2][0-9])|(65[0-4][0-9]{2})|(6[0-4][0-9]{3})|([1-5][0-9]{4})|([1-9][0-9]{0,3})|0)$`),
	}
}

func (v *ArgsValidator) ValidateExposeArgs(tunnelID, address string) error {
	if err := v.ValidateTunnelID(tunnelID); err != nil {
		return err
	}

	if err := v.ValidateAddress(address); err != nil {
		return err
	}

	return nil
}

func (v *ArgsValidator) ValidateTunnelID(tunnelID string) error {
	if !v.regexTunnelID.MatchString(tunnelID) {
		return ErrInvalidTunnelID
	}
	return nil
}

func (v *ArgsValidator) ValidateAddress(address string) error {
	if !v.regexAddress.MatchString(address) {
		return ErrInvalidAddress
	}
	return nil
}
