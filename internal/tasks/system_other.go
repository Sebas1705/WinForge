//go:build !windows

package tasks

import "errors"

type osSystem struct{}

// NewSystem is a machine with no scheduled tasks: this feature is Windows-only.
func NewSystem() System { return osSystem{} }

func (osSystem) List() ([]Raw, error)                  { return nil, nil }
func (osSystem) LastResults([]string) map[string]int32 { return nil }
func (osSystem) CurrentUser() []string                 { return nil }
func (osSystem) Getenv(string) string                  { return "" }
func (osSystem) SetEnabled(string, bool) error {
	return errors.New("scheduled tasks can only be changed on Windows")
}
