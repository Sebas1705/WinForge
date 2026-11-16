//go:build !windows

package sched

import "fmt"

type otherSystem struct{}

// NewSystem is a machine with no Windows tasks: scheduled updates are Windows-only.
func NewSystem() System { return otherSystem{} }

func (otherSystem) Query() (TaskInfo, error) { return TaskInfo{}, nil }
func (otherSystem) Create(Task) error        { return fmt.Errorf("%w", ErrUnsupported) }
func (otherSystem) Delete() error            { return nil }
func (otherSystem) Toast(Toast) error        { return fmt.Errorf("%w", ErrUnsupported) }
func (otherSystem) RegisterProtocol(string) error {
	return fmt.Errorf("%w", ErrUnsupported)
}
func (otherSystem) UnregisterProtocol() error { return nil }

// SystemLanguage is unknown off Windows.
func SystemLanguage() string { return "" }

// ToastScript does not exist off Windows.
func ToastScript() string { return "" }
