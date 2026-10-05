package api

import "encoding/json"

// MarshalJSON preserves the task text and the established terminal-safe bidi
// representation on the public task page.
func (p TaskPage) MarshalJSON() ([]byte, error) {
	type plain TaskPage
	body, err := json.Marshal(plain(p))
	if err != nil {
		return nil, err
	}
	return EscapeBidiJSON(body), nil
}
