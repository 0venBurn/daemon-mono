package edit

import (
	"fmt"
	"strings"

	"github.com/0venburn/daemon/internal/protocol"
)

func ApplyExact(original string, edits []protocol.EditReplacement) (string, error) {
	result := original

	for _, e := range edits {
		count := strings.Count(result, e.OldText)
		if count == 0 {
			return "", fmt.Errorf("old text not found")
		}
		if count > 1 {
			return "", fmt.Errorf("old text matched multiple times")
		}
		result = strings.Replace(result, e.OldText, e.NewText, 1)
	}
	return result, nil
}
