package config

import (
	"fmt"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

func expandScalars(node *yaml.Node, path []string, lookupEnv func(string) (string, bool)) error {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			if err := expandScalars(child, path, lookupEnv); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		for index := 0; index+1 < len(node.Content); index += 2 {
			key := node.Content[index]
			value := node.Content[index+1]
			if err := expandScalars(value, append(path, key.Value), lookupEnv); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			if err := expandScalars(child, path, lookupEnv); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		if node.Tag != "!!str" || !strings.Contains(node.Value, "${") {
			return nil
		}
		expanded, err := expandString(node.Value, configPath(path), lookupEnv)
		if err != nil {
			return err
		}
		node.Value = expanded
	}
	return nil
}

func expandString(value, path string, lookupEnv func(string) (string, bool)) (string, error) {
	var builder strings.Builder
	for rest := value; len(rest) > 0; {
		start := strings.Index(rest, "${")
		if start < 0 {
			builder.WriteString(rest)
			break
		}
		builder.WriteString(rest[:start])
		rest = rest[start+2:]
		end := strings.IndexByte(rest, '}')
		if end < 0 {
			return "", newConfigError(path, "malformed environment placeholder", nil)
		}
		name := rest[:end]
		if !validEnvironmentName(name) {
			return "", newConfigError(path, "malformed environment placeholder", nil)
		}
		if lookupEnv == nil {
			return "", newConfigError(path, fmt.Sprintf("environment variable %s is not set", name), nil)
		}
		resolved, ok := lookupEnv(name)
		if !ok {
			return "", newConfigError(path, fmt.Sprintf("environment variable %s is not set", name), nil)
		}
		builder.WriteString(resolved)
		rest = rest[end+1:]
	}
	return builder.String(), nil
}

func validEnvironmentName(name string) bool {
	if name == "" {
		return false
	}
	for index, character := range name {
		if character == '_' || unicode.IsLetter(character) && character <= unicode.MaxASCII {
			continue
		}
		if index > 0 && unicode.IsDigit(character) && character <= unicode.MaxASCII {
			continue
		}
		return false
	}
	return true
}

func configPath(path []string) string {
	if len(path) == 0 {
		return "document"
	}
	return strings.Join(path, ".")
}
