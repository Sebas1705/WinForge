package main

import (
	"github.com/Sebas1705/WinForge/internal/system"
	"github.com/Sebas1705/WinForge/internal/winfeatures"
)

func (a *App) featuresSystem() winfeatures.System {
	if a.featuresSys != nil {
		return a.featuresSys
	}
	return winfeatures.NewSystem()
}

// FeaturesList reports whether WSL, Virtual Machine Platform, Hyper-V, Windows Sandbox and
// .NET Framework 3.5 are on. It only reads: turning one on or off is a recipe (feature-<id>-on and
// feature-<id>-off) that the page runs through the normal install runner.
func (a *App) FeaturesList() ([]winfeatures.Feature, error) {
	elevated := a.elevated
	if elevated == nil {
		elevated = system.IsElevated
	}
	return winfeatures.List(a.featuresSystem(), elevated()), nil
}

// WSLInfo lists the WSL distributions and the default WSL version. It never starts a distribution
// or runs anything inside one.
func (a *App) WSLInfo() (winfeatures.WSLInfo, error) {
	return winfeatures.WSL(a.featuresSystem()), nil
}
