package eggvars

import (
	"fmt"
	"strconv"
	"strings"
)

func Validate(name, value, rules string) error {
	if rules == "" {
		return nil
	}
	parts := strings.Split(rules, "|")

	required := false
	for _, p := range parts {
		if p == "required" {
			required = true
		}
	}

	if value == "" {
		if required {
			return fmt.Errorf("%s is required", name)
		}
		return nil
	}

	for _, p := range parts {
		rule, param, _ := strings.Cut(p, ":")
		switch rule {
		case "required", "string", "nullable", "sometimes":
		case "integer":
			if _, err := strconv.Atoi(value); err != nil {
				return fmt.Errorf("%s must be an integer", name)
			}
		case "numeric":
			if _, err := strconv.ParseFloat(value, 64); err != nil {
				return fmt.Errorf("%s must be a number", name)
			}
		case "boolean":
			switch strings.ToLower(value) {
			case "true", "false", "1", "0":
			default:
				return fmt.Errorf("%s must be true or false", name)
			}
		case "max":
			if n, err := strconv.Atoi(param); err == nil && len(value) > n {
				return fmt.Errorf("%s must be at most %d characters", name, n)
			}
		case "min":
			if n, err := strconv.Atoi(param); err == nil && len(value) < n {
				return fmt.Errorf("%s must be at least %d characters", name, n)
			}
		case "in":
			allowed := strings.Split(param, ",")
			ok := false
			for _, a := range allowed {
				if value == a {
					ok = true
					break
				}
			}
			if !ok {
				return fmt.Errorf("%s must be one of: %s", name, param)
			}
		}
	}
	return nil
}
