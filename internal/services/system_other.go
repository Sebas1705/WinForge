//go:build !windows

package services

import "errors"

type osSystem struct{}

// NewSystem is a machine with no services: this feature is Windows-only.
func NewSystem() System { return osSystem{} }

func (osSystem) List() ([]Raw, error)                  { return nil, nil }
func (osSystem) States([]string) map[string]State      { return nil }
func (osSystem) Publishers([]string) map[string]string { return nil }
func (osSystem) Getenv(string) string                  { return "" }
func (osSystem) SetStartMode(string, Mode) error {
	return errors.New("services can only be changed on Windows")
}
