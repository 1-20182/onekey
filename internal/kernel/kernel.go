// Package kernel installs the bundled OpenSteamTools kernel into the Steam
// root directory. The three DLLs are embedded at build time so end users need
// no remote download.
package kernel

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed files/*.dll
var files embed.FS

// Names lists the DLLs OpenSteamTools requires in the Steam root.
var Names = []string{"dwmapi.dll", "xinput1_4.dll", "OpenSteamTool.dll"}

// Install copies the embedded OpenSteamTools DLLs into the Steam root so the
// kernel proxies load when Steam starts. Returns the written absolute paths.
func Install(steamPath string) ([]string, error) {
	var written []string
	for _, name := range Names {
		// embed.FS keys paths with "/" only; filepath.Join would build
		// "files\dwmapi.dll" on Windows and never match. Use forward slashes.
		data, err := fs.ReadFile(files, "files/"+name)
		if err != nil {
			return written, fmt.Errorf("read embedded %s: %w", name, err)
		}
		dst := filepath.Join(steamPath, name)
		if err := os.WriteFile(dst, data, 0644); err != nil {
			return written, fmt.Errorf("write %s to steam dir: %w", name, err)
		}
		written = append(written, dst)
	}
	return written, nil
}