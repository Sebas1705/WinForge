package main

import (
	"testing"

	"github.com/Sebas1705/WinForge/internal/winfeatures"
)

// fakeFeatures is a PC with WSL on, Hyper-V off and no Windows Sandbox (a Home edition).
type fakeFeatures struct{ pendingAsked bool }

func (f *fakeFeatures) OptionalFeatures() (map[string]int, error) {
	return map[string]int{"Microsoft-Windows-Subsystem-Linux": 1, "VirtualMachinePlatform": 1, "Microsoft-Hyper-V-All": 2, "NetFx3": 2}, nil
}
func (f *fakeFeatures) PendingFeatures() (map[string]string, error) {
	f.pendingAsked = true
	return map[string]string{"NetFx3": winfeatures.PendingEnable}, nil
}
func (f *fakeFeatures) WSL(args ...string) ([]byte, int, error) {
	return nil, 0, winfeatures.ErrWSLMissing
}
func (f *fakeFeatures) WSLDefaultVersion() (int, bool) { return 1, true }

func TestFeaturesListUsesTheInjectedSystemAndElevation(t *testing.T) {
	s, p := okStores(t)
	a := newApp(emptyCatalog(), s, p)
	f := &fakeFeatures{}
	a.featuresSys = f
	a.elevated = func() bool { return false }
	list, err := a.FeaturesList()
	if err != nil || len(list) != 5 {
		t.Fatalf("%v %v", list, err)
	}
	if list[0].ID != "wsl" || list[0].State != winfeatures.On || !list[0].NeedsAdmin || f.pendingAsked {
		t.Errorf("not elevated: %+v pendingAsked=%v", list[0], f.pendingAsked)
	}
	if list[3].State != winfeatures.Unavailable {
		t.Errorf("no sandbox on this edition: %+v", list[3])
	}
	a.elevated = func() bool { return true }
	list, _ = a.FeaturesList()
	if list[0].NeedsAdmin || !f.pendingAsked || list[4].State != winfeatures.On || !list[4].RestartPending {
		t.Errorf("elevated: %+v", list)
	}
}

func TestWSLInfoUsesTheInjectedSystem(t *testing.T) {
	s, p := okStores(t)
	a := newApp(emptyCatalog(), s, p)
	a.featuresSys = &fakeFeatures{}
	info, err := a.WSLInfo()
	if err != nil || info.DefaultVersion != 1 || len(info.Distros) != 0 || info.Installed {
		t.Fatalf("%+v %v", info, err)
	}
}
