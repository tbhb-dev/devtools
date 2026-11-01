//go:build windows

// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package ghpost

import "os"

const dirSyncSupported = false

// Windows doesn't allow syncing a directory handle, so the receipt file's
// own Sync is the strongest durability available there.
func syncDir(_ string, _ func(*os.File) error) error {
	return nil
}
