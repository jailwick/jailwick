//go:build !linux

package agent

// changeWaiter looks every WatchEvery where there's no inotify (the agent runs on Linux; this
// keeps the package building and testable elsewhere).
var changeWaiter = timerWaiter
