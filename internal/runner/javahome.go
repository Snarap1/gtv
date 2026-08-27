package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
)

type JDK struct {
	Home  string
	Major int
}

func FindJavaHome(minMajor int) (JDK, error) {
	if home := os.Getenv("JAVA_HOME"); home != "" {
		if jdk, ok := inspect(home); ok && jdk.Major >= minMajor {
			return jdk, nil
		}
	}

	var best JDK
	for _, dir := range candidateDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			home := filepath.Join(dir, entry.Name())
			if runtime.GOOS == "darwin" {
				if _, err := os.Stat(filepath.Join(home, "Contents", "Home")); err == nil {
					home = filepath.Join(home, "Contents", "Home")
				}
			}
			if jdk, ok := inspect(home); ok && jdk.Major >= minMajor && jdk.Major > best.Major {
				best = jdk
			}
		}
	}
	if best.Home == "" {
		return JDK{}, fmt.Errorf("no JDK %d+ with javac found (set JAVA_HOME)", minMajor)
	}
	return best, nil
}

func candidateDirs() []string {
	home, _ := os.UserHomeDir()
	var dirs []string
	switch runtime.GOOS {
	case "windows":
		dirs = []string{
			`C:\Program Files\Java`,
			`C:\Program Files\Eclipse Adoptium`,
			`C:\Program Files\Microsoft`,
			filepath.Join(home, "scoop", "apps"),
		}
	case "darwin":
		dirs = []string{"/Library/Java/JavaVirtualMachines"}
	default:
		dirs = []string{"/usr/lib/jvm", "/usr/java", "/opt/java"}
	}
	if home != "" {
		dirs = append(dirs,
			filepath.Join(home, ".sdkman", "candidates", "java"),
			filepath.Join(home, ".asdf", "installs", "java"),
			filepath.Join(home, ".jdks"),
		)
	}
	return dirs
}

var releaseVersion = regexp.MustCompile(`JAVA_VERSION="?(\d+)`)

func inspect(home string) (JDK, bool) {
	javac := filepath.Join(home, "bin", "javac")
	if runtime.GOOS == "windows" {
		javac += ".exe"
	}
	if info, err := os.Stat(javac); err != nil || info.IsDir() {
		return JDK{}, false
	}
	data, err := os.ReadFile(filepath.Join(home, "release"))
	if err != nil {
		return JDK{}, false
	}
	m := releaseVersion.FindSubmatch(data)
	if m == nil {
		return JDK{}, false
	}
	major, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return JDK{}, false
	}

	if resolved, err := filepath.EvalSymlinks(home); err == nil {
		home = resolved
	}
	return JDK{Home: home, Major: major}, true
}

func FindGradleRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, wrapperName())); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no %s found in %s or any parent", wrapperName(), start)
		}
		dir = parent
	}
}

func wrapperName() string {
	if runtime.GOOS == "windows" {
		return "gradlew.bat"
	}
	return "gradlew"
}

func wrapperCommand(root string) (string, []string) {
	path := filepath.Join(root, wrapperName())
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/c", path}
	}
	return path, nil
}
