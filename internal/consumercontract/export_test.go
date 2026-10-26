// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumercontract

// Applies exposes the entry applicability check to the external tests.
func Applies(entry Entry, conf string, target *Document) bool {
	return applies(entry, conf, target)
}

// Edit exposes the conf editing step to the external tests.
func Edit(conf string, entry Entry, target *Document) string {
	return edit(conf, entry, target)
}
