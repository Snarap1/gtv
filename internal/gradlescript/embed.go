package gradlescript

import (
	_ "embed"
	"os"
	"path/filepath"
)

//go:embed listener.gradle
var Listener string

func Materialize(dir string) (string, error) {
	path := filepath.Join(dir, "gtv-listener.gradle")
	if err := os.WriteFile(path, []byte(Listener), 0o644); err != nil {
		return "", err
	}
	return filepath.Abs(path)
}
