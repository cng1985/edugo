package knowledge

import "encoding/json"

func jsonDecode(b []byte, dest any) error { return json.Unmarshal(b, dest) }
