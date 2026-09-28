// Package comment implements the `bh pr comment` command group: list, add,
// reply, delete, resolve, and reopen pull request comments.
package comment

import (
	"fmt"
	"strconv"
	"strings"
)

// parseCommentID parses a comment id argument (bare number or "#123").
func parseCommentID(arg string) (int, error) {
	s := strings.TrimPrefix(strings.TrimSpace(arg), "#")
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid comment id %q", arg)
	}
	return n, nil
}
