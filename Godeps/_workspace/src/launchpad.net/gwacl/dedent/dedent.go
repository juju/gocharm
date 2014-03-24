// Copyright 2013 Canonical Ltd.  This software is licensed under the
// GNU Lesser General Public License version 3 (see the file COPYING).

package dedent

import (
    "regexp"
    "strings"
)

const emptyString = ""

var reLine = regexp.MustCompile(`(?m-s)^.*$`)

// Split the given text into lines.
func splitLines(text string) []string {
    return reLine.FindAllString(text, -1)
}

// Match leading whitespace or tabs. \p{Zs} is a Unicode character class:
// http://en.wikipedia.org/wiki/Mapping_of_Unicode_characters#General_Category
var reLeadingWhitespace = regexp.MustCompile(`^[\p{Zs}\t]+`)

// Find the longest leading margin common between the given lines.
func calculateMargin(lines []string) string {
    var margin string
    var first bool = true
    for _, line := range lines {
        indent := reLeadingWhitespace.FindString(line)
        switch {
        case len(indent) == len(line):
            // The line is either empty or whitespace and will be ignored for
            // the purposes of calculating the margin.
        case first:
            // This is the first line with an indent, so start from here.
            margin = indent
            first = false
        case strings.HasPrefix(indent, margin):
            // This line's indent is longer or equal to the margin. The
            // current margin remains unalterered.
        case strings.HasPrefix(margin, indent):
            // This line's indent is compatible with the margin but shorter
            // (strictly it could be equal, however that condition is handled
            // earlier in this switch). The current indent becomes the margin.
            margin = indent
        default:
            // There is no common margin so stop scanning.
            return emptyString
        }
    }
    return margin
}

// Remove a prefix from each line, if present.
func trimPrefix(lines []string, prefix string) {
    trim := len(prefix)
    for i, line := range lines {
        if strings.HasPrefix(line, prefix) {
            lines[i] = line[trim:]
        }
    }
}

func Dedent(text string) string {
    lines := splitLines(text)
    trimPrefix(lines, calculateMargin(lines))
    return strings.Join(lines, "\n")
}
