// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghpost

import (
	"fmt"
	"os"
	"runtime"
)

const dirSyncSupported = runtime.GOOS != "windows"

func syncDir(dir string, syncFile func(*os.File) error) error {
	// Windows refuses directory handle syncs; the receipt file sync is the
	// strongest durability available there.
	if !dirSyncSupported {
		return nil
	}
	parent, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open directory: %w", err)
	}
	syncErr := syncFile(parent)
	closeErr := parent.Close()
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return fmt.Errorf("close directory: %w", closeErr)
	}
	return nil
}
