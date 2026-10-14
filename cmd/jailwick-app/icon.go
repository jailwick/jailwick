package main

import _ "embed"

// The icons are drawn by tools/icons (node tools/icons/icons.mjs, then render.mjs): the agent's
// green dot in a cell, behind four bars.

// trayIcon is the menu bar icon: a black template image that macOS tints for light and dark menu
// bars. 44×44 for Retina.
//
//go:embed icons/tray.png
var trayIcon []byte

// The app icon (Dock, About box) in two looks: night for a dark appearance, day for a light one.
var (
	//go:embed icons/app-night.png
	appIconNight []byte
	//go:embed icons/app-day.png
	appIconDay []byte
)

// appIcon returns the app icon that matches the system appearance.
func appIcon(dark bool) []byte {
	if dark {
		return appIconNight
	}
	return appIconDay
}
