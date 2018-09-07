package snapshot

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"

	"../convert"
	"../db"
	"../db/coding"
	"../db/ns"
	"../logs"
	"../utils"
)

const currentHeaderVersion = "1"

func SaveSnapshot(snapshotDir string, timestamp int64, filename string) error {
	logs.Log.Noticef("Saving snapshot (%v) into %v...", timestamp, snapshotDir)
	utils.CreateDirectory(snapshotDir)

	timestampString := strconv.FormatInt(int64(timestamp), 10)
	if len(filename) == 0 {
		filename = config.GetString("snapshots.filename")
	}
	if len(filename) == 0 {
		filename = timestampString + ".snap"
	}

	savepth := path.Join(snapshotDir, filename)
	pth := savepth + "_"
	file, err := os.Create(pth)
	if err != nil {
		logs.Log.Noticef("Could not create snapshot file: %v", pth)
		return err
	}
	defer file.Close()

	w := bufio.NewWriter(file)

	var lineBuffer []string

	// Write header
	fmt.Fprintln(w, currentHeaderVersion+","+timestampString)

	var addToBuffer = func(line string) {
		if lowEndDevice {
			fmt.Fprintln(w, line)
		} else {
			lineBuffer = append(lineBuffer, line)
		}
	}

	var commitBuffer = func() {
		defer func() { lineBuffer = nil }()
		if lowEndDevice || lineBuffer == nil {
			return
		}
		sort.Strings(lineBuffer)
		for _, line := range lineBuffer {
			fmt.Fprintln(w, line)
		}
	}

	err = db.Singleton.View(func(tx db.Transaction) error {
		err := coding.ForPrefixInt64(tx, ns.Prefix(ns.NamespaceSnapshotBalance), false, func(key []byte, value int64) (bool, error) {
			// Do not save zero-value addresses
			if value == 0 {
				return true, nil
			}

			line := convert.BytesToTrytes(key[1:])[:81] + ";" + strconv.FormatInt(value, 10)
			addToBuffer(line)

			return true, nil
		})
		if err != nil {
			return err
		}

		commitBuffer()
		return nil
	})

	fmt.Fprintln(w, SNAPSHOT_SEPARATOR)
	err = db.Singleton.View(func(tx db.Transaction) error {
		ns.ForNamespace(tx, ns.NamespaceSnapshotSpent, false, func(key, _ []byte) (bool, error) {
			line := convert.BytesToTrytes(key[1:])[:81]
			addToBuffer(line)
			return true, nil
		})
		commitBuffer()
		return nil
	})
	if err != nil {
		return err
	}

	fmt.Fprintln(w, SNAPSHOT_SEPARATOR)
	err = db.Singleton.View(func(tx db.Transaction) error {
		ns.ForNamespace(tx, ns.NamespacePendingBundle, false, func(key, _ []byte) (bool, error) {
			line := convert.BytesToTrytes(key)
			addToBuffer(line)
			return true, nil
		})
		commitBuffer()
		return nil
	})
	if err != nil {
		return err
	}

	fmt.Fprintln(w, SNAPSHOT_SEPARATOR)
	err = db.Singleton.View(func(tx db.Transaction) error {
		ns.ForNamespace(tx, ns.NamespaceSnapshotted, false, func(key, _ []byte) (bool, error) {
			line := convert.BytesToTrytes(key)
			addToBuffer(line)
			return true, nil
		})
		commitBuffer()
		return nil
	})
	if err != nil {
		return err
	}

	logs.Log.Notice("Snapshot saved, flushing...")
	if err = w.Flush(); err != nil {
		return err
	}
	return os.Rename(pth, savepth)
}
