package report

import (
	"encoding/json"
	"io"
)

// WriteJSON emits the full report as indented JSON.
func WriteJSON(w io.Writer, r *Report) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
