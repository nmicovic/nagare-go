// Copyright 2013 Ato Araki. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package clipboard reads and writes text on the system clipboard.
package clipboard

// ReadAll reads text from the clipboard.
func ReadAll() (string, error) {
	initialize()
	return readAll()
}

// WriteAll writes text to the clipboard.
func WriteAll(text string) error {
	initialize()
	return writeAll(text)
}

// Unsupported reports whether clipboard access is unavailable. On Unix it is
// populated lazily when ReadAll or WriteAll first needs the clipboard.
var Unsupported bool
