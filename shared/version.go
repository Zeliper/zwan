// Package shared holds types and utilities common to the zwan server and agent.
//
// Naming: the *technical* identity is stable — module "github.com/Zeliper/zwan",
// binaries "zwan-server"/"zwan-agent". The *brand* (user-facing product name) is
// intentionally decoupled into ProductName so it can be renamed at any time.
package shared

// ProductName is the user-facing brand name and the single source of truth for it.
// Rename the product at any time by changing this one value (or override at build
// time with: go build -ldflags "-X github.com/Zeliper/zwan/shared.ProductName=NewName").
// Anything shown to users (logs, GUI titles, tray, default adapter/namespace names)
// must reference this instead of hardcoding a name.
var ProductName = "MyWAN"

// Version is the current build version. Override at release time via -ldflags.
var Version = "0.0.0-dev"

// Component identifies which zwan binary is running (stable, machine-facing).
type Component string

const (
	ComponentServer Component = "zwan-server"
	ComponentAgent  Component = "zwan-agent"
)
