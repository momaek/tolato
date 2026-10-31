package localguard

import "errors"

func PrivateDirectory(dir string) (string, error) {
	return "", errors.New("root-only local interfaces are unavailable on Windows")
}
