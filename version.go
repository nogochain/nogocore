// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2026 NogoChain Contributors
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"strings"
)

// semanticAlphabet defines valid characters for semantic version strings.
const semanticAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-"

// Application version constants following semantic versioning 2.0.0
// (http://semver.org/).
const (
	appMajor uint = 1
	appMinor uint = 0
	appPatch uint = 0

	// appPreRelease MUST only contain characters from semanticAlphabet
	// per the semantic versioning spec.
	appPreRelease = "alpha"
)

// appBuild is defined as a variable so it can be overridden during the build
// process with '-ldflags "-X main.appBuild foo"' if needed. It MUST only
// contain characters from semanticAlphabet per the semantic versioning spec.
var appBuild string

// version returns the application version as a properly formed string per the
// semantic versioning 2.0.0 spec (http://semver.org/).
func version() string {
	// Start with the major, minor, and patch versions.
	version := fmt.Sprintf("%d.%d.%d", appMajor, appMinor, appPatch)

	// Append pre-release version if there is one.
	preRelease := normalizeVerString(appPreRelease)
	if preRelease != "" {
		version = fmt.Sprintf("%s-%s", version, preRelease)
	}

	// Append build metadata if there is any.
	build := normalizeVerString(appBuild)
	if build != "" {
		version = fmt.Sprintf("%s+%s", version, build)
	}

	return version
}

// normalizeVerString returns the passed string stripped of all characters which
// are not valid according to the semantic versioning guidelines for pre-release
// version and build metadata strings.
func normalizeVerString(str string) string {
	var result bytes.Buffer
	for _, r := range str {
		if strings.ContainsRune(semanticAlphabet, r) {
			result.WriteRune(r)
		}
	}
	return result.String()
}
