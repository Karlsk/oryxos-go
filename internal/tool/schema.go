package tool

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
)

var supportedSchemaKeywords = map[string]struct{}{
	"type":                 {},
	"properties":           {},
	"required":             {},
	"additionalProperties": {},
	"items":                {},
	"enum":                 {},
}

func validateArguments(schemaDocument json.RawMessage, arguments string) error {
	var schema map[string]any
	if err := decodeOneJSON(schemaDocument, &schema); err != nil {
		return fmt.Errorf("invalid tool schema: %w", err)
	}
	if err := validateSchema(schema, true); err != nil {
		return fmt.Errorf("invalid tool schema: %w", err)
	}
	var value any
	if err := decodeOneJSON([]byte(arguments), &value); err != nil {
		return fmt.Errorf("invalid tool arguments: %w", err)
	}
	if err := validateValue(schema, value, "$arguments"); err != nil {
		return fmt.Errorf("invalid tool arguments: %w", err)
	}
	return nil
}

func decodeOneJSON(data []byte, destination any) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return fmt.Errorf("document is empty")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func validateSchema(schema map[string]any, root bool) error {
	for keyword := range schema {
		if _, supported := supportedSchemaKeywords[keyword]; !supported {
			return fmt.Errorf("unsupported keyword %q", keyword)
		}
	}
	typeName, ok := schema["type"].(string)
	if !ok {
		return fmt.Errorf("type must be a string")
	}
	if root && typeName != "object" {
		return fmt.Errorf("root type must be object")
	}
	switch typeName {
	case "object":
		if raw, exists := schema["properties"]; exists {
			properties, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("properties must be an object")
			}
			for name, rawProperty := range properties {
				property, ok := rawProperty.(map[string]any)
				if !ok {
					return fmt.Errorf("property %q must be a schema object", name)
				}
				if err := validateSchema(property, false); err != nil {
					return fmt.Errorf("property %q: %w", name, err)
				}
			}
		}
		if raw, exists := schema["required"]; exists {
			required, ok := raw.([]any)
			if !ok {
				return fmt.Errorf("required must be an array")
			}
			for _, item := range required {
				if _, ok := item.(string); !ok {
					return fmt.Errorf("required values must be strings")
				}
			}
		}
		if raw, exists := schema["additionalProperties"]; exists {
			allowed, ok := raw.(bool)
			if !ok || allowed {
				return fmt.Errorf("additionalProperties only supports false")
			}
		}
	case "array":
		raw, exists := schema["items"]
		if !exists {
			return fmt.Errorf("array items schema is required")
		}
		items, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("items must be a schema object")
		}
		if err := validateSchema(items, false); err != nil {
			return fmt.Errorf("items: %w", err)
		}
	case "string", "number", "integer", "boolean", "null":
	default:
		return fmt.Errorf("unsupported type %q", typeName)
	}
	if raw, exists := schema["enum"]; exists {
		if _, ok := raw.([]any); !ok {
			return fmt.Errorf("enum must be an array")
		}
	}
	return nil
}

func validateValue(schema map[string]any, value any, path string) error {
	if choices, ok := schema["enum"].([]any); ok {
		matched := false
		for _, choice := range choices {
			if reflect.DeepEqual(choice, value) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%s is not an allowed enum value", path)
		}
	}

	typeName := schema["type"].(string)
	switch typeName {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		properties, _ := schema["properties"].(map[string]any)
		if required, ok := schema["required"].([]any); ok {
			for _, rawName := range required {
				name := rawName.(string)
				if _, exists := object[name]; !exists {
					return fmt.Errorf("%s.%s is required", path, name)
				}
			}
		}
		if additional, exists := schema["additionalProperties"]; exists && additional == false {
			for name := range object {
				if _, declared := properties[name]; !declared {
					return fmt.Errorf("%s.%s is not allowed", path, name)
				}
			}
		}
		for name, rawProperty := range properties {
			if propertyValue, exists := object[name]; exists {
				if err := validateValue(rawProperty.(map[string]any), propertyValue, path+"."+name); err != nil {
					return err
				}
			}
		}
	case "array":
		array, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		items := schema["items"].(map[string]any)
		for index, item := range array {
			if err := validateValue(items, item, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
		}
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s must be a string", path)
		}
	case "number":
		if _, ok := value.(json.Number); !ok {
			return fmt.Errorf("%s must be a number", path)
		}
	case "integer":
		number, ok := value.(json.Number)
		if !ok {
			return fmt.Errorf("%s must be an integer", path)
		}
		if _, err := number.Int64(); err != nil {
			return fmt.Errorf("%s must be an integer", path)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s must be a boolean", path)
		}
	case "null":
		if value != nil {
			return fmt.Errorf("%s must be null", path)
		}
	}
	return nil
}
