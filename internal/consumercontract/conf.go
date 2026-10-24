// SPDX-License-Identifier: Apache-2.0
// Copyright Tony Burns

package consumercontract

import "strings"

// confValue reads a key from conf the way the shell reader does: the first
// line outside a comment whose text before the first = is the key.
func confValue(conf, name string) (string, bool) {
	for line := range strings.SplitSeq(conf, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if key, value, found := strings.Cut(line, "="); found && strings.TrimSpace(key) == name {
			return strings.TrimSpace(value), true
		}
	}
	return "", false
}

// applies says whether a consumer in the state conf describes must act on
// the entry. The target contract lists which values it still accepts.
func applies(entry Entry, conf string, target *Document) bool {
	value, present := confValue(conf, entry.Key)
	switch entry.AppliesWhen {
	case WhenKeyAbsent:
		return !present
	default:
		if !present {
			return false
		}
		key, kept := target.Lookup(entry.File, entry.Key)
		return !kept || !key.AcceptsValue(value)
	}
}

// edit performs one entry on conf and leaves every other line alone. An
// entry whose key stays in the target sets it; one whose key is gone
// removes it.
func edit(conf string, entry Entry, target *Document) string {
	_, kept := target.Lookup(entry.File, entry.Key)
	lines := strings.Split(conf, "\n")
	replaced := false
	out := make([]string, 0, len(lines)+1)
	for _, line := range lines {
		key, _, found := strings.Cut(line, "=")
		if !found || strings.HasPrefix(strings.TrimSpace(key), "#") || strings.TrimSpace(key) != entry.Key {
			out = append(out, line)
			continue
		}
		if kept && !replaced {
			out = append(out, entry.Key+" = "+entry.TargetValue)
			replaced = true
		}
	}
	result := strings.Join(out, "\n")
	if kept && !replaced {
		if result != "" && !strings.HasSuffix(result, "\n") {
			result += "\n"
		}
		result += entry.Key + " = " + entry.TargetValue + "\n"
	}
	return result
}
