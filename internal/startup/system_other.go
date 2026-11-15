//go:build !windows

package startup

import "errors"

type osSystem struct{}

// NewSystem is a machine with nothing that starts with Windows: this feature is Windows-only.
func NewSystem() System { return osSystem{} }

func (osSystem) Entries(Origin) ([]Raw, error)              { return nil, nil }
func (osSystem) Approved(Origin) (map[string][]byte, error) { return nil, nil }
func (osSystem) SetApproved(Origin, string, []byte) error {
	return errors.New("startup items can only be changed on Windows")
}
func (osSystem) Exists(string) bool                   { return false }
func (osSystem) Getenv(string) string                 { return "" }
func (osSystem) Inspect([]string) map[string]FileInfo { return nil }
