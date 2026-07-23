package urllifecycle

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"unicode/utf8"
)

type normalizedRef struct {
	ref       LocalRef
	key       string
	namespace Namespace
	query     string
	identity  string
}

type normalizedLookup struct {
	ref      normalizedRef
	rawQuery string
	extras   string
}

func (catalog *Catalog) normalizeLocalRef(value LocalRef) (normalizedRef, error) {
	if catalog == nil {
		return normalizedRef{}, invalid("catalog", "is required")
	}
	path, err := normalizePath(value.Path, catalog.limits.MaxPathBytes)
	if err != nil {
		return normalizedRef{}, invalid("path", "%v", err)
	}
	namespace, err := catalog.namespaceForPath(path)
	if err != nil {
		return normalizedRef{}, err
	}
	values := make(map[string]string, len(value.Query))
	for index, item := range value.Query {
		key := strings.TrimSpace(item.Key)
		if _, exists := namespace.querySet[key]; !exists {
			return normalizedRef{}, invalid("query", "item %d key %q is not an identity key for namespace %q", index, key, namespace.key)
		}
		if _, exists := values[key]; exists {
			return normalizedRef{}, invalid("query", "contains duplicate identity key %q", key)
		}
		values[key] = item.Value
	}
	return namespace.normalizedRef(path, values)
}

func (catalog *Catalog) normalizeLookup(value Lookup) (normalizedLookup, error) {
	if catalog == nil {
		return normalizedLookup{}, invalid("catalog", "is required")
	}
	path, err := normalizePath(value.EscapedPath, catalog.limits.MaxPathBytes)
	if err != nil {
		return normalizedLookup{}, invalid("path", "%v", err)
	}
	if len(value.RawQuery) > catalog.limits.MaxQueryBytes {
		return normalizedLookup{}, invalid("query", "exceeds %d bytes", catalog.limits.MaxQueryBytes)
	}
	namespace, err := catalog.namespaceForPath(path)
	if err != nil {
		return normalizedLookup{}, err
	}
	values := make(map[string]string, len(namespace.queryKeys))
	extras := make([]string, 0)
	if value.RawQuery != "" {
		segments := strings.Split(value.RawQuery, "&")
		for _, segment := range segments {
			if segment == "" {
				extras = append(extras, segment)
				continue
			}
			rawKey := segment
			rawValue := ""
			if at := strings.IndexByte(segment, '='); at >= 0 {
				rawKey, rawValue = segment[:at], segment[at+1:]
			}
			key, err := url.QueryUnescape(rawKey)
			if err != nil {
				return normalizedLookup{}, invalid("query", "contains malformed key encoding")
			}
			position, identity := namespace.querySet[key]
			if !identity {
				extras = append(extras, segment)
				continue
			}
			if _, exists := values[key]; exists {
				return normalizedLookup{}, invalid("query", "contains duplicate identity key %q", key)
			}
			decoded, err := url.QueryUnescape(rawValue)
			if err != nil {
				return normalizedLookup{}, invalid("query", "identity key %q contains malformed encoding", key)
			}
			values[namespace.queryKeys[position].key] = decoded
		}
	}
	ref, err := namespace.normalizedRef(path, values)
	if err != nil {
		return normalizedLookup{}, err
	}
	return normalizedLookup{ref: ref, rawQuery: value.RawQuery, extras: strings.Join(extras, "&")}, nil
}

func (namespace compiledNamespace) normalizedRef(path string, values map[string]string) (normalizedRef, error) {
	all := make([]QueryValue, 0, len(namespace.queryKeys))
	emitted := make([]QueryValue, 0, len(namespace.queryKeys))
	for _, definition := range namespace.queryKeys {
		value, exists := values[definition.key]
		if !exists {
			value = definition.defaultVal
		}
		all = append(all, QueryValue{Key: definition.key, Value: value})
		if !definition.omitDefault || value != definition.defaultVal {
			emitted = append(emitted, QueryValue{Key: definition.key, Value: value})
		}
	}
	query := encodeQuery(emitted)
	if len(query) > namespace.maxBytes {
		return normalizedRef{}, invalid("query", "exceeds namespace limit of %d bytes", namespace.maxBytes)
	}
	identity := encodeQuery(all)
	return normalizedRef{
		ref:       LocalRef{Path: path, Query: emitted},
		key:       path + "\x00" + string(namespace.key) + "\x00" + identity,
		namespace: namespace.key,
		query:     query,
		identity:  identity,
	}, nil
}

func encodeQuery(values []QueryValue) string {
	if len(values) == 0 {
		return ""
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, url.QueryEscape(value.Key)+"="+url.QueryEscape(value.Value))
	}
	return strings.Join(parts, "&")
}

func (catalog *Catalog) namespaceForPath(path string) (compiledNamespace, error) {
	for _, namespace := range catalog.namespaces {
		if namespace.pathPrefix == "/" ||
			path == namespace.pathPrefix ||
			strings.HasPrefix(path, namespace.pathPrefix+"/") {
			return namespace, nil
		}
	}
	return compiledNamespace{}, invalid("path", "does not match a configured namespace")
}

func normalizePath(raw string, maxBytes int) (string, error) {
	if raw == "" || raw[0] != '/' {
		return "", fmt.Errorf("must be an absolute path")
	}
	if len(raw) > maxBytes {
		return "", fmt.Errorf("exceeds %d bytes", maxBytes)
	}
	if strings.HasPrefix(raw, "//") {
		return "", fmt.Errorf("must not be a scheme-relative reference")
	}
	if strings.ContainsAny(raw, "?#\\") {
		return "", fmt.Errorf("must not contain query, fragment, or backslash")
	}
	if !utf8.ValidString(raw) {
		return "", fmt.Errorf("must be valid UTF-8")
	}
	var builder strings.Builder
	builder.Grow(len(raw))
	for index := 0; index < len(raw); index++ {
		value := raw[index]
		if value < 0x20 || value == 0x7f {
			return "", fmt.Errorf("contains a control character")
		}
		if value != '%' {
			builder.WriteByte(value)
			continue
		}
		if index+2 >= len(raw) {
			return "", fmt.Errorf("contains malformed percent encoding")
		}
		high, okHigh := fromHex(raw[index+1])
		low, okLow := fromHex(raw[index+2])
		if !okHigh || !okLow {
			return "", fmt.Errorf("contains malformed percent encoding")
		}
		decoded := high<<4 | low
		if decoded == 0 {
			return "", fmt.Errorf("contains NUL")
		}
		if isUnreserved(decoded) {
			builder.WriteByte(decoded)
		} else {
			builder.WriteByte('%')
			builder.WriteByte(toHex(high))
			builder.WriteByte(toHex(low))
		}
		index += 2
	}
	normalized := removeDotSegments(builder.String())
	if normalized == "" {
		normalized = "/"
	}
	if normalized[0] != '/' || strings.HasPrefix(normalized, "//") {
		return "", fmt.Errorf("normalizes to an unsafe path")
	}
	return normalized, nil
}

func removeDotSegments(input string) string {
	output := ""
	for input != "" {
		switch {
		case strings.HasPrefix(input, "../"):
			input = input[3:]
		case strings.HasPrefix(input, "./"):
			input = input[2:]
		case strings.HasPrefix(input, "/./"):
			input = "/" + input[3:]
		case input == "/.":
			input = "/"
		case strings.HasPrefix(input, "/../"):
			input = "/" + input[4:]
			output = removeLastSegment(output)
		case input == "/..":
			input = "/"
			output = removeLastSegment(output)
		case input == "." || input == "..":
			input = ""
		default:
			count := strings.IndexByte(input[1:], '/')
			if input[0] != '/' {
				count = strings.IndexByte(input, '/')
				if count < 0 {
					output += input
					input = ""
					continue
				}
				output += input[:count]
				input = input[count:]
				continue
			}
			if count < 0 {
				output += input
				input = ""
				continue
			}
			count++
			output += input[:count]
			input = input[count:]
		}
	}
	return output
}

func removeLastSegment(value string) string {
	if value == "" {
		return ""
	}
	at := strings.LastIndexByte(value, '/')
	if at < 0 {
		return ""
	}
	return value[:at]
}

func fromHex(value byte) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return value - '0', true
	case value >= 'a' && value <= 'f':
		return value - 'a' + 10, true
	case value >= 'A' && value <= 'F':
		return value - 'A' + 10, true
	default:
		return 0, false
	}
}

func toHex(value byte) byte {
	if value < 10 {
		return '0' + value
	}
	return 'A' + value - 10
}

func isUnreserved(value byte) bool {
	return value >= 'a' && value <= 'z' ||
		value >= 'A' && value <= 'Z' ||
		value >= '0' && value <= '9' ||
		slices.Contains([]byte{'-', '.', '_', '~'}, value)
}
