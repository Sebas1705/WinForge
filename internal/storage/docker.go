package storage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"
)

// dockerUnits are the multipliers Docker prints sizes with. Docker counts in powers of 1000.
var dockerUnits = map[string]float64{
	"b": 1, "kb": 1e3, "mb": 1e6, "gb": 1e9, "tb": 1e12, "pb": 1e15,
}

// parseSize reads a size as Docker prints it ("1.2GB", "300MB", "12.5kB", "0B") into bytes.
// Anything else, a trailing "(45%)" included, is not a size.
func parseSize(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	i := 0
	for i < len(s) && (s[i] == '.' || (s[i] >= '0' && s[i] <= '9')) {
		i++
	}
	if i == 0 || i == len(s) {
		return 0, false
	}
	n, err := strconv.ParseFloat(s[:i], 64)
	if err != nil || n < 0 {
		return 0, false
	}
	mult, ok := dockerUnits[strings.ToLower(s[i:])]
	if !ok {
		return 0, false
	}
	return int64(n * mult), true
}

// dockerDF reads `docker system df`: one JSON object per line.
type dfRow struct {
	Type        string `json:"Type"`
	TotalCount  string `json:"TotalCount"`
	Reclaimable string `json:"Reclaimable"`
}

// dockerBuildCache returns how much the build cache could free. ok is false when Docker is not
// installed or its engine is not running: then the category is not offered at all.
func dockerBuildCache(ctx context.Context, r Runner) (bytes_ int64, count int, ok bool) {
	if r == nil {
		return 0, 0, false
	}
	out, err := r.Run(ctx, "docker", "system", "df", "--format", "{{json .}}")
	if err != nil {
		return 0, 0, false
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var row dfRow
		if json.Unmarshal([]byte(line), &row) != nil || row.Type != "Build Cache" {
			continue
		}
		// "Reclaimable" reads "1.2GB (100%)": the size is the first word.
		first, _, _ := strings.Cut(strings.TrimSpace(row.Reclaimable), " ")
		n, good := parseSize(first)
		if !good {
			return 0, 0, true // the engine answered, with something that is not a size: nothing to offer
		}
		c, _ := strconv.Atoi(strings.TrimSpace(row.TotalCount))
		return n, c, true
	}
	return 0, 0, true
}

// dockerDanglingImages returns the size and number of untagged images nothing refers to.
func dockerDanglingImages(ctx context.Context, r Runner) (bytes_ int64, count int, ok bool) {
	if r == nil {
		return 0, 0, false
	}
	out, err := r.Run(ctx, "docker", "images", "--filter", "dangling=true", "--format", "{{.Size}}")
	if err != nil {
		return 0, 0, false
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if n, good := parseSize(line); good {
			bytes_ += n
			count++
		}
	}
	return bytes_, count, true
}
