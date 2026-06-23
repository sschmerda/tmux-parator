package sessionconfig

import (
	"fmt"
	"strings"
)

type condition struct {
	left     string
	operator string
	right    string
}

func validateCondition(expression string) error {
	if strings.TrimSpace(expression) == "" {
		return nil
	}
	_, err := parseCondition(expression)
	return err
}

func evaluateCondition(expression string) (bool, error) {
	if strings.TrimSpace(expression) == "" {
		return true, nil
	}
	parsed, err := parseCondition(expression)
	if err != nil {
		return false, err
	}
	switch parsed.operator {
	case "==":
		return parsed.left == parsed.right, nil
	case "!=":
		return parsed.left != parsed.right, nil
	default:
		return false, fmt.Errorf("unsupported operator %q", parsed.operator)
	}
}

func parseCondition(expression string) (condition, error) {
	expression = strings.TrimSpace(expression)
	operatorIndex := -1
	operator := ""
	var quote byte
	escaped := false
	for i := 0; i < len(expression); i++ {
		char := expression[i]
		if escaped {
			escaped = false
			continue
		}
		if char == '\\' && quote != 0 {
			escaped = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		if i+1 >= len(expression) {
			continue
		}
		candidate := expression[i : i+2]
		if candidate != "==" && candidate != "!=" {
			continue
		}
		if operatorIndex >= 0 {
			return condition{}, fmt.Errorf("must contain exactly one == or != comparison")
		}
		operatorIndex = i
		operator = candidate
		i++
	}
	if quote != 0 {
		return condition{}, fmt.Errorf("contains an unterminated quoted value")
	}
	if operatorIndex < 0 {
		return condition{}, fmt.Errorf("must contain an == or != comparison")
	}
	left, err := conditionOperand(expression[:operatorIndex])
	if err != nil {
		return condition{}, fmt.Errorf("left operand: %w", err)
	}
	right, err := conditionOperand(expression[operatorIndex+2:])
	if err != nil {
		return condition{}, fmt.Errorf("right operand: %w", err)
	}
	return condition{left: left, operator: operator, right: right}, nil
}

func conditionOperand(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("is empty")
	}
	if value[0] != '\'' && value[0] != '"' {
		if strings.ContainsAny(value, "'\"") {
			return "", fmt.Errorf("contains an unmatched quote")
		}
		return value, nil
	}
	quote := value[0]
	if len(value) < 2 || value[len(value)-1] != quote {
		return "", fmt.Errorf("contains an unmatched quote")
	}
	value = value[1 : len(value)-1]
	value = strings.ReplaceAll(value, `\`+string(quote), string(quote))
	value = strings.ReplaceAll(value, `\\`, `\`)
	return value, nil
}
