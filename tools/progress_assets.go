// Package progresstools publishes the same writer used by repository entries
// as inline execution material for projects that have not integrated it yet.
package progresstools

import _ "embed"

//go:embed testing/RuntimeProgress.ps1
var PowerShellWriter string
