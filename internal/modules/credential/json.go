package credential

import "encoding/json"

func jsonUnmarshalStd(b []byte, dest any) error { return json.Unmarshal(b, dest) }
