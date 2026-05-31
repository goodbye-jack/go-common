package sms

import (
	"strconv"
	"strings"
)

func chooseNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func intToString(value int) string {
	return strconv.Itoa(value)
}

func renderTemplate(template string, variables map[string]string) string {
	result := template
	for key, value := range variables {
		result = strings.ReplaceAll(result, "{{"+key+"}}", value)
		result = strings.ReplaceAll(result, "${"+key+"}", value)
		result = strings.ReplaceAll(result, "#"+key+"#", value)
	}
	return strings.TrimSpace(result)
}
