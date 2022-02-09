package memcached

import (
	"encoding/json"
	"fmt"

	"github.com/couchbase/gomemcached"
)

type mcOpcode struct {
	code gomemcached.CommandCode
	name string
}

func (m *mcOpcode) UnmarshalJSON(bytes []byte) error {
	var name string
	if err := json.Unmarshal(bytes, &name); err != nil {
		return err
	}
	for code, opName := range gomemcached.CommandNames {
		if opName == name {
			m.code = code
			m.name = name
			return nil
		}
	}
	return fmt.Errorf("unknown memcached opcode %s", name)
}
