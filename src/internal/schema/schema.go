package schema

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type Validator struct {
	schema *jsonschema.Schema
}

// Compile errors on an invalid schema, since that is a caller bug, not a model failure.
func Compile(name string, raw json.RawMessage) (*Validator, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("schema: %q is not valid JSON: %w", name, err)
	}
	compiler := jsonschema.NewCompiler()
	url := name + ".json"
	if err := compiler.AddResource(url, doc); err != nil {
		return nil, fmt.Errorf("schema: %q could not be added: %w", name, err)
	}
	compiled, err := compiler.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("schema: %q is not a valid JSON Schema: %w", name, err)
	}
	return &Validator{schema: compiled}, nil
}

func (v *Validator) Validate(content json.RawMessage) error {
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf("content is not valid JSON: %w", err)
	}
	if err := v.schema.Validate(instance); err != nil {
		return fmt.Errorf("content does not match schema: %w", err)
	}
	return nil
}
