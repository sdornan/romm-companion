// Package process reports whether Steam is running.
//
// Steam only reads shortcuts.vdf at startup and rewrites it on exit, so a
// write while it is running is silently lost. Every apply is gated on this.
package process

// Running reports whether a Steam client process is alive.
var Running = running
