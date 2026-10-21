package section

import (
	"os"
	"regexp"
	"strconv"
)

// NextNumbered scans dirAbs for filenames matching re (which must have exactly
// one capture group around the number) and returns one past the highest number
// found. It returns 1 if the directory doesn't exist or has no matching files.
func NextNumbered(dirAbs string, re *regexp.Regexp) (int, error) {
	entries, err := os.ReadDir(dirAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return 1, nil
		}
		return 0, err
	}

	max := 0
	for _, e := range entries {
		m := re.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if n > max {
			max = n
		}
	}
	return max + 1, nil
}
