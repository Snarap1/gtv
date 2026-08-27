// Package gradlescript carries the Gradle init script that streams test events.
package gradlescript

import (
	_ "embed"
	"os"
	"path/filepath"
)

//go:embed listener.gradle
var Listener string

// Materialize writes the init script into dir and returns its absolute path.
func Materialize(dir string) (string, error) {
	path := filepath.Join(dir, "gtv-listener.gradle")
	if err := os.WriteFile(path, []byte(Listener), 0o644); err != nil {
		return "", err
	}
	return filepath.Abs(path)
}
