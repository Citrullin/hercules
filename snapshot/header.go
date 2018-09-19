package snapshot

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gitlab.com/semkodev/hercules/logs"
)

type SnapshotHeader struct {
	Version   int64
	Timestamp int64
}

func LoadHeader(path string) (*SnapshotHeader, error) {
	f, err := os.OpenFile(path, os.O_RDONLY, os.ModePerm)
	if err != nil {
		logs.Log.Fatalf("open file error: %v", err)
		return nil, err
	}
	defer f.Close()

	rd := bufio.NewReader(f)
	line, err := rd.ReadString('\n')
	line = strings.TrimSpace(line)
	tokens := strings.Split(line, ",")

	// Pre-header version. Use filename as timestamp
	if len(tokens) < 2 {
		filename := filepath.Base(path)
		timestamp, err := strconv.ParseInt(strings.Split(filename, ".")[0], 10, 32)
		if err != nil || timestamp < TIMESTAMP_MIN || timestamp > time.Now().Unix() {
			return nil, errors.New("no header found and filename seems not to be a timestamp")
		}
		return &SnapshotHeader{0, timestamp}, nil
	}

	version, err := strconv.ParseInt(tokens[0], 10, 32)

	if err != nil {
		return nil, err
	}

	var timestamp int64 = 0

	if version > 0 {
		timestamp, err = strconv.ParseInt(tokens[1], 10, 32)
		if err != nil || timestamp < TIMESTAMP_MIN || timestamp > time.Now().Unix() {
			return nil, errors.New("header validation failed")
		}
	}

	if timestamp == 0 {
		return nil, errors.New("unknown header version found (hercules upgrade necessary?)")
	}

	return &SnapshotHeader{version, timestamp}, nil
}
