//go:build !windows

package winfeatures

import "errors"

type osSystem struct{}

// NewSystem is a machine without Windows features: this page is Windows-only.
func NewSystem() System { return osSystem{} }

func (osSystem) OptionalFeatures() (map[string]int, error) {
	return nil, errors.New("Windows features can only be read on Windows")
}
func (osSystem) PendingFeatures() (map[string]string, error) {
	return nil, errors.New("Windows features can only be read on Windows")
}
func (osSystem) WSL(...string) ([]byte, int, error) { return nil, 0, ErrWSLMissing }
func (osSystem) WSLDefaultVersion() (int, bool)     { return 0, false }
