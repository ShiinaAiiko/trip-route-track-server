package methods

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

func MinifyJSON(src string) string {
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, []byte(src)); err != nil {
		return src // 如果解析失败，原样返回
	}
	return buffer.String()
}

func FastFixJSON(raw string) string {
	re := regexp.MustCompile(`,(\s*[}\]])`)

	raw = re.ReplaceAllString(raw, "$1")

	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(raw, "{") {
		return raw
	}

	var stack []rune
	inString := false
	isEscaped := false

	// 遍历一次，找出所有未闭合的符号
	for _, char := range raw {
		if isEscaped {
			isEscaped = false
			continue
		}
		if char == '\\' {
			isEscaped = true
			continue
		}
		if char == '"' {
			inString = !inString
			continue
		}

		if !inString {
			if char == '{' || char == '[' {
				stack = append(stack, char)
			} else if char == '}' || char == ']' {
				if len(stack) > 0 {
					// 这里简化处理，假设输出是合规的，只弹出对应的
					stack = stack[:len(stack)-1]
				}
			}
		}
	}

	// 处理断在字符串内部的情况
	if inString {
		raw += `"`
	}

	// 逆序闭合所有栈内符号
	for i := len(stack) - 1; i >= 0; i-- {
		// 补全前先处理可能多余的逗号
		raw = strings.TrimSuffix(raw, ",")
		if stack[i] == '{' {
			raw += "}"
		} else if stack[i] == '[' {
			raw += "]"
		}
	}

	return raw
}
