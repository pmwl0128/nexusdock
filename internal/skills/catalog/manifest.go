package catalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	ManifestAPIVersionV1 = "agentdock.io/v1"
	ManifestKindSkill    = "Skill"
)

var (
	skillNamePattern    = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]{0,62}[a-z0-9])?$`)
	operationIDPattern  = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)
	semverPattern       = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
	allowedRunnerValues = map[string]struct{}{"command": {}, "python": {}, "node": {}, "go": {}, "prompt": {}}
)

// Manifest is the AgentDock Skill package manifest. JSON field names are also
// the canonical YAML keys, which keeps contract generation deterministic.
type Manifest struct {
	APIVersion string           `json:"apiVersion"`
	Kind       string           `json:"kind"`
	Metadata   ManifestMetadata `json:"metadata"`
	Spec       ManifestSpec     `json:"spec"`
}

type ManifestMetadata struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName,omitempty"`
	Version     string   `json:"version"`
	Description string   `json:"description,omitempty"`
	License     string   `json:"license,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type ManifestSpec struct {
	Operations    []Operation   `json:"operations"`
	Compatibility Compatibility `json:"compatibility,omitempty"`
	Permissions   Permissions   `json:"permissions,omitempty"`
}

type Operation struct {
	ID             string         `json:"id"`
	Description    string         `json:"description,omitempty"`
	Runner         string         `json:"runner"`
	Entrypoint     string         `json:"entrypoint"`
	Args           []string       `json:"args,omitempty"`
	TimeoutSeconds int            `json:"timeoutSeconds,omitempty"`
	InputSchema    map[string]any `json:"inputSchema,omitempty"`
	OutputSchema   map[string]any `json:"outputSchema,omitempty"`
}

type Compatibility struct {
	OS        []string `json:"os,omitempty"`
	Arch      []string `json:"arch,omitempty"`
	AgentDock string   `json:"agentdock,omitempty"`
}

type Permissions struct {
	Network    NetworkPermission    `json:"network,omitempty"`
	Filesystem FilesystemPermission `json:"filesystem,omitempty"`
	Secrets    []SecretPermission   `json:"secrets,omitempty"`
}

type NetworkPermission struct {
	Mode  string   `json:"mode,omitempty"`
	Hosts []string `json:"hosts,omitempty"`
}

type FilesystemPermission struct {
	Read  []string `json:"read,omitempty"`
	Write []string `json:"write,omitempty"`
}

type SecretPermission struct {
	Name     string `json:"name"`
	Required bool   `json:"required,omitempty"`
}

type ValidationIssue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ValidationError struct {
	Issues []ValidationIssue `json:"issues"`
}

func (e *ValidationError) Error() string {
	if len(e.Issues) == 0 {
		return "invalid skill manifest"
	}
	return fmt.Sprintf("invalid skill manifest: %s: %s", e.Issues[0].Path, e.Issues[0].Message)
}

// ParseManifest accepts the strict, dependency-free YAML subset used by
// agentdock.yaml. It intentionally rejects YAML aliases, tags and duplicate
// keys so the same package has the same meaning on every node.
func ParseManifest(data []byte) (Manifest, error) {
	root, err := parseYAML(data)
	if err != nil {
		return Manifest{}, err
	}
	b, err := json.Marshal(root)
	if err != nil {
		return Manifest{}, fmt.Errorf("marshal parsed manifest: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var manifest Manifest
	if err := dec.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode agentdock.yaml: %w", err)
	}
	if err := ValidateManifest(manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func ValidateManifest(m Manifest) error {
	var issues []ValidationIssue
	add := func(path, code, message string) {
		issues = append(issues, ValidationIssue{Path: path, Code: code, Message: message})
	}
	if m.APIVersion != ManifestAPIVersionV1 {
		add("apiVersion", "UNSUPPORTED_VERSION", "must be agentdock.io/v1")
	}
	if m.Kind != ManifestKindSkill {
		add("kind", "INVALID_KIND", "must be Skill")
	}
	if !skillNamePattern.MatchString(m.Metadata.Name) {
		add("metadata.name", "INVALID_NAME", "must be a lowercase package identifier up to 64 characters")
	}
	if !semverPattern.MatchString(m.Metadata.Version) {
		add("metadata.version", "INVALID_VERSION", "must be semantic version MAJOR.MINOR.PATCH")
	}
	if len(m.Spec.Operations) == 0 {
		add("spec.operations", "MISSING_OPERATIONS", "at least one operation is required")
	}
	seenOperations := make(map[string]struct{}, len(m.Spec.Operations))
	for i, op := range m.Spec.Operations {
		base := fmt.Sprintf("spec.operations[%d]", i)
		if !operationIDPattern.MatchString(op.ID) {
			add(base+".id", "INVALID_OPERATION_ID", "must start with a lowercase letter and contain only lowercase letters, digits, dot, underscore or dash")
		}
		if _, ok := seenOperations[op.ID]; ok {
			add(base+".id", "DUPLICATE_OPERATION", "operation id must be unique")
		}
		seenOperations[op.ID] = struct{}{}
		if _, ok := allowedRunnerValues[op.Runner]; !ok {
			add(base+".runner", "INVALID_RUNNER", "must be command, python, node, go or prompt")
		}
		if err := validatePackagePath(op.Entrypoint); err != nil {
			add(base+".entrypoint", "INVALID_ENTRYPOINT", err.Error())
		}
		if op.TimeoutSeconds < 0 || op.TimeoutSeconds > 86400 {
			add(base+".timeoutSeconds", "INVALID_TIMEOUT", "must be between 0 and 86400")
		}
		validateJSONSchemaShape(base+".inputSchema", op.InputSchema, add)
		validateJSONSchemaShape(base+".outputSchema", op.OutputSchema, add)
	}
	for i, value := range m.Spec.Compatibility.OS {
		switch value {
		case "darwin", "linux", "windows":
		default:
			add(fmt.Sprintf("spec.compatibility.os[%d]", i), "INVALID_OS", "must be darwin, linux or windows")
		}
	}
	for i, value := range m.Spec.Compatibility.Arch {
		switch value {
		case "amd64", "arm64":
		default:
			add(fmt.Sprintf("spec.compatibility.arch[%d]", i), "INVALID_ARCH", "must be amd64 or arm64")
		}
	}
	networkMode := m.Spec.Permissions.Network.Mode
	if networkMode == "" {
		networkMode = "none"
	}
	if networkMode != "none" && networkMode != "declared" {
		add("spec.permissions.network.mode", "INVALID_NETWORK_MODE", "must be none or declared")
	}
	if networkMode == "none" && len(m.Spec.Permissions.Network.Hosts) > 0 {
		add("spec.permissions.network.hosts", "HOSTS_WITHOUT_NETWORK", "hosts require network mode declared")
	}
	if networkMode == "declared" && len(m.Spec.Permissions.Network.Hosts) == 0 {
		add("spec.permissions.network.hosts", "MISSING_NETWORK_HOSTS", "declared network mode requires at least one host")
	}
	for i, host := range m.Spec.Permissions.Network.Hosts {
		if strings.ContainsAny(host, "/:@ \\") || strings.TrimSpace(host) != host || host == "" {
			add(fmt.Sprintf("spec.permissions.network.hosts[%d]", i), "INVALID_NETWORK_HOST", "must be a hostname or wildcard hostname without scheme, path or credentials")
		}
	}
	for i, secret := range m.Spec.Permissions.Secrets {
		if !regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,127}$`).MatchString(secret.Name) {
			add(fmt.Sprintf("spec.permissions.secrets[%d].name", i), "INVALID_SECRET_NAME", "must be an uppercase environment-style identifier")
		}
	}
	if len(issues) > 0 {
		sort.SliceStable(issues, func(i, j int) bool { return issues[i].Path < issues[j].Path })
		return &ValidationError{Issues: issues}
	}
	return nil
}

func validateJSONSchemaShape(path string, schema map[string]any, add func(string, string, string)) {
	if len(schema) == 0 {
		return
	}
	typeValue, ok := schema["type"]
	if !ok {
		add(path+".type", "MISSING_SCHEMA_TYPE", "JSON Schema root must declare type")
		return
	}
	if _, ok := typeValue.(string); !ok {
		add(path+".type", "INVALID_SCHEMA_TYPE", "JSON Schema type must be a string")
	}
}

func validatePackagePath(value string) error {
	if value == "" {
		return errors.New("must not be empty")
	}
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "\\") || strings.Contains(value, "\\") {
		return errors.New("must be a slash-separated relative package path")
	}
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return errors.New("must not contain empty, dot or parent segments")
		}
	}
	return nil
}

// MarshalManifest emits deterministic YAML without aliases or implicit types.
func MarshalManifest(m Manifest) ([]byte, error) {
	if err := ValidateManifest(m); err != nil {
		return nil, err
	}
	var b strings.Builder
	writeKV(&b, 0, "apiVersion", m.APIVersion)
	writeKV(&b, 0, "kind", m.Kind)
	b.WriteString("metadata:\n")
	writeKV(&b, 2, "name", m.Metadata.Name)
	writeKV(&b, 2, "displayName", m.Metadata.DisplayName)
	writeKV(&b, 2, "version", m.Metadata.Version)
	writeKV(&b, 2, "description", m.Metadata.Description)
	writeKV(&b, 2, "license", m.Metadata.License)
	writeStringList(&b, 2, "tags", m.Metadata.Tags)
	b.WriteString("spec:\n")
	b.WriteString("  operations:\n")
	for _, op := range m.Spec.Operations {
		writeKVPrefix(&b, 4, "- id", op.ID)
		writeKV(&b, 6, "description", op.Description)
		writeKV(&b, 6, "runner", op.Runner)
		writeKV(&b, 6, "entrypoint", op.Entrypoint)
		writeStringList(&b, 6, "args", op.Args)
		if op.TimeoutSeconds > 0 {
			fmt.Fprintf(&b, "%stimeoutSeconds: %d\n", strings.Repeat(" ", 6), op.TimeoutSeconds)
		}
		writeJSONObject(&b, 6, "inputSchema", op.InputSchema)
		writeJSONObject(&b, 6, "outputSchema", op.OutputSchema)
	}
	if len(m.Spec.Compatibility.OS) > 0 || len(m.Spec.Compatibility.Arch) > 0 || m.Spec.Compatibility.AgentDock != "" {
		b.WriteString("  compatibility:\n")
		writeStringList(&b, 4, "os", m.Spec.Compatibility.OS)
		writeStringList(&b, 4, "arch", m.Spec.Compatibility.Arch)
		writeKV(&b, 4, "agentdock", m.Spec.Compatibility.AgentDock)
	}
	if hasPermissions(m.Spec.Permissions) {
		b.WriteString("  permissions:\n")
		if m.Spec.Permissions.Network.Mode != "" || len(m.Spec.Permissions.Network.Hosts) > 0 {
			b.WriteString("    network:\n")
			writeKV(&b, 6, "mode", m.Spec.Permissions.Network.Mode)
			writeStringList(&b, 6, "hosts", m.Spec.Permissions.Network.Hosts)
		}
		if len(m.Spec.Permissions.Filesystem.Read) > 0 || len(m.Spec.Permissions.Filesystem.Write) > 0 {
			b.WriteString("    filesystem:\n")
			writeStringList(&b, 6, "read", m.Spec.Permissions.Filesystem.Read)
			writeStringList(&b, 6, "write", m.Spec.Permissions.Filesystem.Write)
		}
		if len(m.Spec.Permissions.Secrets) > 0 {
			b.WriteString("    secrets:\n")
			for _, secret := range m.Spec.Permissions.Secrets {
				writeKVPrefix(&b, 6, "- name", secret.Name)
				if secret.Required {
					b.WriteString("        required: true\n")
				}
			}
		}
	}
	return []byte(b.String()), nil
}

func hasPermissions(p Permissions) bool {
	return p.Network.Mode != "" || len(p.Network.Hosts) > 0 || len(p.Filesystem.Read) > 0 || len(p.Filesystem.Write) > 0 || len(p.Secrets) > 0
}

func writeKV(b *strings.Builder, indent int, key, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(b, "%s%s: %s\n", strings.Repeat(" ", indent), key, quoteYAML(value))
}

func writeKVPrefix(b *strings.Builder, indent int, key, value string) {
	fmt.Fprintf(b, "%s%s: %s\n", strings.Repeat(" ", indent), key, quoteYAML(value))
}

func writeStringList(b *strings.Builder, indent int, key string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(b, "%s%s:\n", strings.Repeat(" ", indent), key)
	for _, value := range values {
		fmt.Fprintf(b, "%s- %s\n", strings.Repeat(" ", indent+2), quoteYAML(value))
	}
}

func writeJSONObject(b *strings.Builder, indent int, key string, value map[string]any) {
	if len(value) == 0 {
		return
	}
	encoded, _ := json.Marshal(value)
	fmt.Fprintf(b, "%s%s: %s\n", strings.Repeat(" ", indent), key, encoded)
}

func quoteYAML(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

type yamlLine struct {
	indent int
	text   string
	line   int
}

func parseYAML(data []byte) (map[string]any, error) {
	lines, err := tokenizeYAML(data)
	if err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, errors.New("agentdock.yaml is empty")
	}
	if strings.HasPrefix(lines[0].text, "-") {
		return nil, errors.New("agentdock.yaml root must be a mapping")
	}
	value, next, err := parseYAMLBlock(lines, 0, lines[0].indent)
	if err != nil {
		return nil, err
	}
	if next != len(lines) {
		return nil, fmt.Errorf("line %d: unexpected content", lines[next].line)
	}
	root, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("agentdock.yaml root must be a mapping")
	}
	return root, nil
}

func tokenizeYAML(data []byte) ([]yamlLine, error) {
	rawLines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	result := make([]yamlLine, 0, len(rawLines))
	for i, raw := range rawLines {
		if strings.ContainsRune(raw, '\t') {
			return nil, fmt.Errorf("line %d: tabs are not allowed", i+1)
		}
		trimmedRight := strings.TrimRight(raw, " \r")
		trimmed := strings.TrimSpace(trimmedRight)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || trimmed == "---" || trimmed == "..." {
			continue
		}
		if containsYAMLAnchorAliasOrTag(trimmed) {
			return nil, fmt.Errorf("line %d: YAML aliases, anchors and tags are not supported", i+1)
		}
		indent := len(trimmedRight) - len(strings.TrimLeft(trimmedRight, " "))
		if indent%2 != 0 {
			return nil, fmt.Errorf("line %d: indentation must use multiples of two spaces", i+1)
		}
		result = append(result, yamlLine{indent: indent, text: stripYAMLComment(strings.TrimLeft(trimmedRight, " ")), line: i + 1})
	}
	return result, nil
}

func containsYAMLAnchorAliasOrTag(value string) bool {
	inSingle, inDouble := false, false
	for index := 0; index < len(value); index++ {
		character := value[index]
		switch character {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
			continue
		case '"':
			if !inSingle && (index == 0 || value[index-1] != '\\') {
				inDouble = !inDouble
			}
			continue
		}
		if inSingle || inDouble || (character != '&' && character != '*' && character != '!') {
			continue
		}
		previousIsBoundary := index == 0 || value[index-1] == ' ' || value[index-1] == ':' || value[index-1] == '-'
		if !previousIsBoundary || index+1 >= len(value) {
			continue
		}
		next := value[index+1]
		if (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') || (next >= '0' && next <= '9') || next == '_' {
			return true
		}
	}
	return false
}

func stripYAMLComment(value string) string {
	inSingle, inDouble := false, false
	for i, r := range value {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle && (i == 0 || value[i-1] != '\\') {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble && (i == 0 || value[i-1] == ' ') {
				return strings.TrimSpace(value[:i])
			}
		}
	}
	return strings.TrimSpace(value)
}

func parseYAMLBlock(lines []yamlLine, index, indent int) (any, int, error) {
	if index >= len(lines) {
		return nil, index, errors.New("unexpected end of YAML")
	}
	if lines[index].indent != indent {
		return nil, index, fmt.Errorf("line %d: unexpected indentation", lines[index].line)
	}
	if strings.HasPrefix(lines[index].text, "- ") || lines[index].text == "-" {
		return parseYAMLList(lines, index, indent)
	}
	return parseYAMLMap(lines, index, indent)
}

func parseYAMLMap(lines []yamlLine, index, indent int) (map[string]any, int, error) {
	result := map[string]any{}
	for index < len(lines) && lines[index].indent == indent && !strings.HasPrefix(lines[index].text, "-") {
		line := lines[index]
		key, rest, ok := splitYAMLPair(line.text)
		if !ok || key == "" {
			return nil, index, fmt.Errorf("line %d: expected key: value", line.line)
		}
		if _, exists := result[key]; exists {
			return nil, index, fmt.Errorf("line %d: duplicate key %q", line.line, key)
		}
		index++
		if rest == "" {
			if index >= len(lines) || lines[index].indent <= indent {
				result[key] = map[string]any{}
				continue
			}
			child, next, err := parseYAMLBlock(lines, index, lines[index].indent)
			if err != nil {
				return nil, index, err
			}
			result[key], index = child, next
			continue
		}
		value, err := parseYAMLScalar(rest)
		if err != nil {
			return nil, index, fmt.Errorf("line %d: %w", line.line, err)
		}
		result[key] = value
	}
	return result, index, nil
}

func parseYAMLList(lines []yamlLine, index, indent int) ([]any, int, error) {
	var result []any
	for index < len(lines) && lines[index].indent == indent && (strings.HasPrefix(lines[index].text, "- ") || lines[index].text == "-") {
		line := lines[index]
		rest := strings.TrimSpace(strings.TrimPrefix(line.text, "-"))
		index++
		if rest == "" {
			if index >= len(lines) || lines[index].indent <= indent {
				result = append(result, nil)
				continue
			}
			child, next, err := parseYAMLBlock(lines, index, lines[index].indent)
			if err != nil {
				return nil, index, err
			}
			result, index = append(result, child), next
			continue
		}
		if key, firstRest, ok := splitYAMLPair(rest); ok {
			item := map[string]any{}
			if firstRest == "" {
				if index >= len(lines) || lines[index].indent <= indent {
					item[key] = map[string]any{}
				} else {
					child, next, err := parseYAMLBlock(lines, index, lines[index].indent)
					if err != nil {
						return nil, index, err
					}
					item[key], index = child, next
				}
			} else {
				value, err := parseYAMLScalar(firstRest)
				if err != nil {
					return nil, index, fmt.Errorf("line %d: %w", line.line, err)
				}
				item[key] = value
			}
			if index < len(lines) && lines[index].indent > indent {
				extra, next, err := parseYAMLMap(lines, index, lines[index].indent)
				if err != nil {
					return nil, index, err
				}
				for k, v := range extra {
					if _, exists := item[k]; exists {
						return nil, index, fmt.Errorf("line %d: duplicate key %q", lines[index].line, k)
					}
					item[k] = v
				}
				index = next
			}
			result = append(result, item)
			continue
		}
		value, err := parseYAMLScalar(rest)
		if err != nil {
			return nil, index, fmt.Errorf("line %d: %w", line.line, err)
		}
		result = append(result, value)
	}
	return result, index, nil
}

func splitYAMLPair(value string) (string, string, bool) {
	inSingle, inDouble, depth := false, false, 0
	for i, r := range value {
		switch r {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle && (i == 0 || value[i-1] != '\\') {
				inDouble = !inDouble
			}
		case '[', '{':
			if !inSingle && !inDouble {
				depth++
			}
		case ']', '}':
			if !inSingle && !inDouble {
				depth--
			}
		case ':':
			if !inSingle && !inDouble && depth == 0 {
				return strings.TrimSpace(value[:i]), strings.TrimSpace(value[i+1:]), true
			}
		}
	}
	return "", "", false
}

func parseYAMLScalar(value string) (any, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "{") || strings.HasPrefix(value, "[") {
		var decoded any
		if err := json.Unmarshal([]byte(value), &decoded); err == nil {
			return decoded, nil
		}
		if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
			inner := strings.TrimSpace(value[1 : len(value)-1])
			if inner == "" {
				return []any{}, nil
			}
			parts := strings.Split(inner, ",")
			items := make([]any, 0, len(parts))
			for _, part := range parts {
				item, err := parseYAMLScalar(strings.TrimSpace(part))
				if err != nil {
					return nil, err
				}
				items = append(items, item)
			}
			return items, nil
		}
		return nil, errors.New("inline mappings must use JSON syntax")
	}
	if strings.HasPrefix(value, "\"") {
		var decoded string
		if err := json.Unmarshal([]byte(value), &decoded); err != nil {
			return nil, fmt.Errorf("invalid quoted string: %w", err)
		}
		return decoded, nil
	}
	if strings.HasPrefix(value, "'") {
		if len(value) < 2 || !strings.HasSuffix(value, "'") {
			return nil, errors.New("unterminated single-quoted string")
		}
		return strings.ReplaceAll(value[1:len(value)-1], "''", "'"), nil
	}
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null", "~":
		return nil, nil
	}
	if number, err := strconv.Atoi(value); err == nil {
		return number, nil
	}
	return value, nil
}
