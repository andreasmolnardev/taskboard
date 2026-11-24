package app

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

type vCardDocument struct {
	Properties []vCardProperty
}

type vCardProperty struct {
	Group      string
	Name       string
	Parameters []vCardParameter
	Value      string
}

type vCardParameter struct {
	Name   string
	Values []string
}

func parseVCardDocument(data []byte) (vCardDocument, error) {
	if !utf8.Valid(data) {
		return vCardDocument{}, fmt.Errorf("vCard is not valid UTF-8")
	}
	lines, err := unfoldVCardLines(string(data))
	if err != nil {
		return vCardDocument{}, err
	}
	if len(lines) < 3 || !strings.EqualFold(lines[0], "BEGIN:VCARD") || !strings.EqualFold(lines[len(lines)-1], "END:VCARD") {
		return vCardDocument{}, fmt.Errorf("BEGIN:VCARD and END:VCARD are required")
	}
	card := vCardDocument{Properties: make([]vCardProperty, 0, len(lines)-2)}
	for _, line := range lines[1 : len(lines)-1] {
		if strings.EqualFold(line, "BEGIN:VCARD") || strings.EqualFold(line, "END:VCARD") {
			return vCardDocument{}, fmt.Errorf("nested vCard is not allowed")
		}
		property, err := parseVCardProperty(line)
		if err != nil {
			return vCardDocument{}, err
		}
		card.Properties = append(card.Properties, property)
	}
	if len(propertyValues(card, "VERSION")) == 0 {
		return vCardDocument{}, fmt.Errorf("VERSION is required")
	}
	version := strings.TrimSpace(unescapeVCardText(propertyValues(card, "VERSION")[0].Value))
	if version != "2.1" && version != "3.0" && version != "4.0" {
		return vCardDocument{}, fmt.Errorf("unsupported vCard version %q", version)
	}
	if len(propertyValues(card, "FN")) == 0 || strings.TrimSpace(unescapeVCardText(propertyValues(card, "FN")[0].Value)) == "" {
		return vCardDocument{}, fmt.Errorf("FN is required")
	}
	return card, nil
}

func unfoldVCardLines(value string) ([]string, error) {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	if value == "" {
		return nil, fmt.Errorf("empty vCard")
	}
	raw := strings.Split(value, "\n")
	if len(raw) > 0 && raw[len(raw)-1] == "" {
		raw = raw[:len(raw)-1]
	}
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if len(lines) == 0 {
				return nil, fmt.Errorf("vCard continuation has no property")
			}
			lines[len(lines)-1] += line[1:]
			continue
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func parseVCardProperty(line string) (vCardProperty, error) {
	colon := vCardValueSeparator(line)
	if colon <= 0 {
		return vCardProperty{}, fmt.Errorf("invalid vCard property")
	}
	parts := splitVCardSegments(line[:colon], ';')
	namePart := parts[0]
	group := ""
	if dot := strings.IndexByte(namePart, '.'); dot > 0 {
		group, namePart = namePart[:dot], namePart[dot+1:]
	}
	if namePart == "" {
		return vCardProperty{}, fmt.Errorf("vCard property name is empty")
	}
	property := vCardProperty{Group: group, Name: strings.ToUpper(namePart), Value: line[colon+1:]}
	for _, parameterPart := range parts[1:] {
		if parameterPart == "" {
			return vCardProperty{}, fmt.Errorf("empty vCard parameter")
		}
		equal := strings.IndexByte(parameterPart, '=')
		if equal <= 0 {
			return vCardProperty{}, fmt.Errorf("invalid vCard parameter")
		}
		parameter := vCardParameter{Name: strings.ToUpper(parameterPart[:equal])}
		for _, value := range splitVCardSegments(parameterPart[equal+1:], ',') {
			parameter.Values = append(parameter.Values, unquoteVCardParameter(value))
		}
		if len(parameter.Values) == 0 {
			return vCardProperty{}, fmt.Errorf("vCard parameter has no value")
		}
		property.Parameters = append(property.Parameters, parameter)
	}
	return property, nil
}

func vCardValueSeparator(value string) int {
	quoted := false
	escaped := false
	for index := 0; index < len(value); index++ {
		if escaped {
			escaped = false
			continue
		}
		if value[index] == '\\' {
			escaped = true
			continue
		}
		if value[index] == '"' {
			quoted = !quoted
			continue
		}
		if value[index] == ':' && !quoted {
			return index
		}
	}
	return -1
}

func splitVCardSegments(value string, separator byte) []string {
	var result []string
	start := 0
	quoted := false
	escaped := false
	for index := 0; index < len(value); index++ {
		if escaped {
			escaped = false
			continue
		}
		switch value[index] {
		case '\\':
			escaped = true
		case '"':
			quoted = !quoted
		case separator:
			if !quoted {
				result = append(result, value[start:index])
				start = index + 1
			}
		}
	}
	return append(result, value[start:])
}

func unquoteVCardParameter(value string) string {
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		return strings.ReplaceAll(value[1:len(value)-1], `\"`, `"`)
	}
	return value
}

func propertyValues(card vCardDocument, name string) []vCardProperty {
	result := make([]vCardProperty, 0)
	for _, property := range card.Properties {
		if strings.EqualFold(property.Name, name) {
			result = append(result, property)
		}
	}
	return result
}

func firstVCardValue(card vCardDocument, name string) string {
	values := propertyValues(card, name)
	if len(values) == 0 {
		return ""
	}
	return unescapeVCardText(values[0].Value)
}

func splitVCardStructuredValue(value string) []string {
	var result []string
	start := 0
	escaped := false
	for index := 0; index < len(value); index++ {
		if escaped {
			escaped = false
			continue
		}
		if value[index] == '\\' {
			escaped = true
			continue
		}
		if value[index] == ';' {
			result = append(result, value[start:index])
			start = index + 1
		}
	}
	return append(result, value[start:])
}

func unescapeVCardText(value string) string {
	var result strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] != '\\' || index+1 >= len(value) {
			result.WriteByte(value[index])
			continue
		}
		index++
		switch value[index] {
		case 'n', 'N':
			result.WriteByte('\n')
		case '\\', ';', ',', ':':
			result.WriteByte(value[index])
		default:
			result.WriteByte(value[index])
		}
	}
	return result.String()
}

func escapeVCardText(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, ";", `\;`)
	value = strings.ReplaceAll(value, ",", `\,`)
	value = strings.ReplaceAll(value, "\r\n", `\n`)
	value = strings.ReplaceAll(value, "\n", `\n`)
	value = strings.ReplaceAll(value, "\r", `\n`)
	return value
}

func encodeVCardDocument(card vCardDocument) ([]byte, error) {
	lines := []string{"BEGIN:VCARD"}
	for _, property := range card.Properties {
		name := property.Name
		if property.Group != "" {
			name = property.Group + "." + name
		}
		for _, parameter := range property.Parameters {
			name += ";" + parameter.Name + "="
			values := make([]string, len(parameter.Values))
			for index, value := range parameter.Values {
				if strings.ContainsAny(value, ";,:\"") {
					values[index] = `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
				} else {
					values[index] = value
				}
			}
			name += strings.Join(values, ",")
		}
		lines = append(lines, name+":"+property.Value)
	}
	lines = append(lines, "END:VCARD")
	var output strings.Builder
	for _, line := range lines {
		for _, folded := range foldVCardLine(line) {
			output.WriteString(folded)
			output.WriteString("\r\n")
		}
	}
	return []byte(output.String()), nil
}

func foldVCardLine(line string) []string {
	if len(line) <= 75 {
		return []string{line}
	}
	result := make([]string, 0, 2)
	remaining := line
	first := true
	for len(remaining) > 0 {
		limit := 75
		prefix := ""
		if !first {
			limit = 74
			prefix = " "
		}
		if len(remaining) <= limit {
			result = append(result, prefix+remaining)
			break
		}
		cut := limit
		for cut > 0 && !utf8.ValidString(remaining[:cut]) {
			cut--
		}
		if cut == 0 {
			cut = limit
		}
		result = append(result, prefix+remaining[:cut])
		remaining = remaining[cut:]
		first = false
	}
	return result
}

func projectVCardToContact(contact interface{ Set(string, any) }, card vCardDocument) {
	contact.Set("formatted_name", firstVCardValue(card, "FN"))
	if values := propertyValues(card, "N"); len(values) > 0 {
		parts := splitVCardStructuredValue(values[0].Value)
		if len(parts) > 0 {
			contact.Set("family_name", unescapeVCardText(parts[0]))
		}
		if len(parts) > 1 {
			contact.Set("given_name", unescapeVCardText(parts[1]))
		}
	}
	if values := propertyValues(card, "EMAIL"); len(values) > 0 {
		contact.Set("email", unescapeVCardText(values[0].Value))
	}
	if values := propertyValues(card, "TEL"); len(values) > 0 {
		contact.Set("phone", unescapeVCardText(values[0].Value))
	}
	if values := propertyValues(card, "ORG"); len(values) > 0 {
		contact.Set("organization", unescapeVCardText(values[0].Value))
	}
	if values := propertyValues(card, "TITLE"); len(values) > 0 {
		contact.Set("title", unescapeVCardText(values[0].Value))
	}
	if values := propertyValues(card, "ADR"); len(values) > 0 {
		contact.Set("address", unescapeVCardText(values[0].Value))
	}
	if values := propertyValues(card, "NOTE"); len(values) > 0 {
		contact.Set("notes", unescapeVCardText(values[0].Value))
	}
	if uid := firstVCardValue(card, "UID"); uid != "" {
		contact.Set("uid", uid)
	}
}

func vCardPropertyMatches(property vCardProperty, filter cardDAVPropFilter) bool {
	if filter.IsNotDefined != nil {
		return false
	}
	for _, parameterFilter := range filter.ParamFilters {
		values := make([]string, 0)
		for _, parameter := range property.Parameters {
			if strings.EqualFold(parameter.Name, parameterFilter.Name) {
				values = append(values, parameter.Values...)
			}
		}
		if parameterFilter.IsNotDefined != nil {
			if len(values) != 0 {
				return false
			}
			continue
		}
		if parameterFilter.TextMatch != nil {
			matched := false
			for _, value := range values {
				if cardDAVTextMatchValue(value, *parameterFilter.TextMatch) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		}
	}
	if filter.TextMatch == nil {
		return true
	}
	return cardDAVTextMatchValue(property.Value, *filter.TextMatch)
}

func cardDAVTextMatchValue(value string, match cardDAVTextMatch) bool {
	value = strings.ToLower(unescapeVCardText(value))
	needle := strings.ToLower(unescapeVCardText(match.Value))
	switch strings.ToLower(match.MatchType) {
	case "starts-with":
		return strings.HasPrefix(value, needle)
	case "ends-with":
		return strings.HasSuffix(value, needle)
	default:
		return strings.Contains(value, needle)
	}
}

func sortVCardProperties(properties []vCardProperty) {
	sort.SliceStable(properties, func(left, right int) bool {
		return properties[left].Name < properties[right].Name
	})
}
