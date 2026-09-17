package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

// OverlayIssue is one merge or validation issue, carrying one-based line and
// column coordinates when the issue originates from an overlay document.
type OverlayIssue struct {
	Severity string
	Code     string
	Message  string
	Pointer  *string
	Line     int
	Column   int
}

// OverlayInvalidError reports an overlay document that failed validation or a
// strict-mode target miss. The handler maps it to a 422 with the issue list.
type OverlayInvalidError struct {
	Errors []OverlayIssue
}

// Error implements error using the first issue message.
func (err *OverlayInvalidError) Error() string {
	if len(err.Errors) > 0 {
		return err.Errors[0].Message
	}
	return "invalid overlay"
}

// Unwrap lets errors.Is match ErrValidation for a generic 422 fallback.
func (err *OverlayInvalidError) Unwrap() error { return ErrValidation }

const (
	overlaySchemaKind        = "platform/v1"
	overlayTargetMissLenient = "lenient"
	overlayTargetMissStrict  = "strict"
	overlayInvalidCode       = "overlay_invalid"
)

// platformOverlay is the decoded platform-v1 overlay document.
type platformOverlay struct {
	TargetKind string
	Mode       string
	Actions    []overlayAction
}

// overlayAction is one compiled platform-v1 action in document order.
type overlayAction struct {
	Target string
	Op     string // merge | remove | patch
	Merge  map[string]any
	Patch  []patchOp
	Line   int
	Column int
}

// patchOp is one RFC 6902 operation carried by a patch action.
type patchOp struct {
	Op     string
	Path   string
	From   string
	Value  any
	Line   int
	Column int
}

// ParseOverlay validates and compiles one platform-v1 overlay document. It
// rejects duplicate object keys and structural violations with one-based
// coordinates before any merge is attempted.
func ParseOverlay(content []byte) (map[string]any, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		return nil, overlayInvalidAt(err.Error(), 1, 1)
	}
	if len(root.Content) == 0 {
		return nil, overlayInvalidAt("empty overlay document", 1, 1)
	}
	document := root.Content[0]
	if err := rejectDuplicateOverlayKeys(document, ""); err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := document.Decode(&raw); err != nil {
		return nil, overlayInvalidAt(err.Error(), document.Line, document.Column)
	}
	if _, issue := compilePlatformOverlay(raw); issue != nil {
		return nil, &OverlayInvalidError{Errors: []OverlayIssue{*issue}}
	}
	return raw, nil
}

// ApplyPlatformOverlay merges one validated platform-v1 overlay into a base
// document, returning the merged document and the JSON pointers it wrote in
// document order. Target misses are warnings in lenient mode and errors in
// strict mode. The base document is never mutated.
func ApplyPlatformOverlay(base map[string]any, raw map[string]any) (merged map[string]any, pointers []string, warnings []OverlayIssue, err error) {
	overlay, issue := compilePlatformOverlay(raw)
	if issue != nil {
		return nil, nil, nil, &OverlayInvalidError{Errors: []OverlayIssue{*issue}}
	}
	mode := overlay.Mode
	if mode == "" {
		mode = overlayTargetMissLenient
	}
	result := deepCopy(base).(map[string]any)
	written := map[string]bool{}
	order := make([]string, 0)
	for _, action := range overlay.Actions {
		touched, actionWarnings, actionErr := applyOverlayAction(result, action, mode)
		warnings = append(warnings, actionWarnings...)
		if actionErr != nil {
			return nil, nil, warnings, actionErr
		}
		for _, pointer := range touched {
			if !written[pointer] {
				written[pointer] = true
				order = append(order, pointer)
			}
		}
	}
	return result, order, warnings, nil
}

// CanonicalJSON serializes a decoded document deterministically using sorted
// object keys, satisfying the RFC 8785 JCS canonicalization used by the merge
// engine and provenance hashes.
func CanonicalJSON(value any) ([]byte, error) { return json.Marshal(value) }

// CanonicalHash returns the lowercase SHA-256 of the canonical JSON form.
func CanonicalHash(value any) (string, error) {
	encoded, err := CanonicalJSON(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// DeepCopy returns an independent copy of a decoded document so the base is
// never mutated by an overlay application.
func DeepCopy(value any) any { return deepCopy(value) }

func deepCopy(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		copy := make(map[string]any, len(typed))
		for key, child := range typed {
			copy[key] = deepCopy(child)
		}
		return copy
	case []any:
		copy := make([]any, len(typed))
		for index, child := range typed {
			copy[index] = deepCopy(child)
		}
		return copy
	default:
		return typed
	}
}

func compilePlatformOverlay(raw map[string]any) (platformOverlay, *OverlayIssue) {
	overlay := platformOverlay{Mode: overlayTargetMissLenient}
	kind, _ := raw["overlay"].(string)
	if kind != overlaySchemaKind {
		return overlay, &OverlayIssue{Severity: "error", Code: overlayInvalidCode, Message: fmt.Sprintf("overlay must be %q", overlaySchemaKind), Line: 1, Column: 1}
	}
	targetKind, _ := raw["target_kind"].(string)
	if targetKind == "" {
		return overlay, &OverlayIssue{Severity: "error", Code: overlayInvalidCode, Message: "target_kind is required", Line: 1, Column: 1}
	}
	overlay.TargetKind = targetKind
	if mode, ok := raw["mode"].(string); ok {
		if mode != overlayTargetMissLenient && mode != overlayTargetMissStrict {
			return overlay, &OverlayIssue{Severity: "error", Code: overlayInvalidCode, Message: "mode must be lenient or strict", Line: 1, Column: 1}
		}
		overlay.Mode = mode
	}
	actionsRaw, ok := raw["actions"].([]any)
	if !ok || len(actionsRaw) == 0 {
		return overlay, &OverlayIssue{Severity: "error", Code: overlayInvalidCode, Message: "actions must be a non-empty array", Line: 1, Column: 1}
	}
	overlay.Actions = make([]overlayAction, 0, len(actionsRaw))
	for _, entry := range actionsRaw {
		action, actionIssue := compileOverlayAction(entry)
		if actionIssue != nil {
			return overlay, actionIssue
		}
		overlay.Actions = append(overlay.Actions, action)
	}
	return overlay, nil
}

func compileOverlayAction(entry any) (overlayAction, *OverlayIssue) {
	actionMap, ok := entry.(map[string]any)
	if !ok {
		return overlayAction{}, &OverlayIssue{Severity: "error", Code: overlayInvalidCode, Message: "each action must be an object", Line: 1, Column: 1}
	}
	action := overlayAction{}
	hasTarget := actionMap["target"] != nil
	hasMerge := actionMap["merge"] != nil
	hasRemove := actionMap["remove"] != nil
	hasPatch := actionMap["patch"] != nil
	switch {
	case hasMerge && !hasRemove && !hasPatch && hasTarget:
		target, ok := actionMap["target"].(string)
		if !ok || target == "" {
			return overlayAction{}, &OverlayIssue{Severity: "error", Code: overlayInvalidCode, Message: "merge target must be a non-empty string", Line: 1, Column: 1}
		}
		action.Op = "merge"
		action.Target = target
		mergeValue, ok := actionMap["merge"].(map[string]any)
		if !ok {
			return overlayAction{}, &OverlayIssue{Severity: "error", Code: overlayInvalidCode, Message: "merge value must be an object", Line: 1, Column: 1}
		}
		action.Merge = mergeValue
	case hasRemove && !hasMerge && !hasPatch && hasTarget:
		target, ok := actionMap["target"].(string)
		if !ok || target == "" {
			return overlayAction{}, &OverlayIssue{Severity: "error", Code: overlayInvalidCode, Message: "remove target must be a non-empty string", Line: 1, Column: 1}
		}
		action.Op = "remove"
		action.Target = target
	case hasPatch && !hasMerge && !hasRemove && !hasTarget:
		action.Op = "patch"
		patchList, ok := actionMap["patch"].([]any)
		if !ok || len(patchList) == 0 {
			return overlayAction{}, &OverlayIssue{Severity: "error", Code: overlayInvalidCode, Message: "patch must be a non-empty array", Line: 1, Column: 1}
		}
		for _, opEntry := range patchList {
			opMap, ok := opEntry.(map[string]any)
			if !ok {
				return overlayAction{}, &OverlayIssue{Severity: "error", Code: overlayInvalidCode, Message: "each patch operation must be an object", Line: 1, Column: 1}
			}
			op := patchOp{Op: opMap["op"].(string), Path: opMap["path"].(string), Value: opMap["value"]}
			if from, ok := opMap["from"].(string); ok {
				op.From = from
			}
			if !validPatchOp(op) {
				return overlayAction{}, &OverlayIssue{Severity: "error", Code: overlayInvalidCode, Message: fmt.Sprintf("invalid patch operation %q", op.Op), Line: 1, Column: 1}
			}
			action.Patch = append(action.Patch, op)
		}
	default:
		return overlayAction{}, &OverlayIssue{Severity: "error", Code: overlayInvalidCode, Message: "action must be exactly one of merge, remove, or patch", Line: 1, Column: 1}
	}
	return action, nil
}

func validPatchOp(op patchOp) bool {
	switch op.Op {
	case "add":
		return true
	case "replace", "test":
		return op.Path != ""
	case "remove":
		return op.Path != ""
	case "move", "copy":
		return op.From != "" && op.Path != ""
	default:
		return false
	}
}

func applyOverlayAction(document map[string]any, action overlayAction, mode string) ([]string, []OverlayIssue, error) {
	switch action.Op {
	case "merge":
		return applyOverlayMerge(document, action, mode)
	case "remove":
		return applyOverlayRemove(document, action, mode)
	case "patch":
		return applyOverlayPatch(document, action)
	}
	return nil, nil, nil
}

func applyOverlayMerge(document map[string]any, action overlayAction, mode string) ([]string, []OverlayIssue, error) {
	paths, err := expandSelector(document, action.Target)
	if err != nil {
		return nil, nil, err
	}
	if len(paths) == 0 {
		if mode == overlayTargetMissStrict {
			return nil, nil, overlayInvalidAt(fmt.Sprintf("merge target %q not found", action.Target), action.Line, action.Column)
		}
		return nil, []OverlayIssue{{Severity: "warning", Code: "target_miss", Message: fmt.Sprintf("merge target %q not found", action.Target), Line: action.Line, Column: action.Column}}, nil
	}
	for _, path := range paths {
		value, ok := getNode(document, path)
		if !ok {
			if mode == overlayTargetMissStrict {
				return nil, nil, overlayInvalidAt(fmt.Sprintf("merge target %q not found", action.Target), action.Line, action.Column)
			}
			continue
		}
		object, isObject := value.(map[string]any)
		if !isObject {
			if mode == overlayTargetMissStrict {
				return nil, nil, overlayInvalidAt(fmt.Sprintf("merge target %q is not an object", action.Target), action.Line, action.Column)
			}
			continue
		}
		mergeObject(object, action.Merge)
	}
	return []string{action.Target}, nil, nil
}

func applyOverlayRemove(document map[string]any, action overlayAction, mode string) ([]string, []OverlayIssue, error) {
	paths, err := expandSelector(document, action.Target)
	if err != nil {
		return nil, nil, err
	}
	if len(paths) == 0 {
		if mode == overlayTargetMissStrict {
			return nil, nil, overlayInvalidAt(fmt.Sprintf("remove target %q not found", action.Target), action.Line, action.Column)
		}
		return nil, []OverlayIssue{{Severity: "warning", Code: "target_miss", Message: fmt.Sprintf("remove target %q not found", action.Target), Line: action.Line, Column: action.Column}}, nil
	}
	// Array deletions run in reverse document order so indices stay valid.
	sort.SliceStable(paths, func(i, j int) bool {
		return len(paths[i]) > len(paths[j]) || (len(paths[i]) == len(paths[j]) && paths[i][len(paths[i])-1] > paths[j][len(paths[j])-1])
	})
	for _, path := range paths {
		if _, removed, removeErr := removeNode(document, path); removeErr != nil || !removed {
			if mode == overlayTargetMissStrict {
				return nil, nil, overlayInvalidAt(fmt.Sprintf("remove target %q not found", action.Target), action.Line, action.Column)
			}
			continue
		}
	}
	return []string{action.Target}, nil, nil
}

func applyOverlayPatch(document map[string]any, action overlayAction) ([]string, []OverlayIssue, error) {
	written := make([]string, 0, len(action.Patch))
	for _, op := range action.Patch {
		if err := applyPatchOperation(document, op); err != nil {
			return nil, nil, err
		}
		if op.Op != "test" {
			written = append(written, op.Path)
		}
	}
	return written, nil, nil
}

// mergeObject implements RFC 7386 merge: null removes, objects recurse, and
// other values overwrite. The target map is mutated in place.
func mergeObject(target map[string]any, patch map[string]any) {
	for key, value := range patch {
		if value == nil {
			delete(target, key)
			continue
		}
		child, isObject := value.(map[string]any)
		existing, existingIsObject := target[key].(map[string]any)
		if isObject && existingIsObject {
			mergeObject(existing, child)
			continue
		}
		target[key] = deepCopy(value)
	}
}

// expandSelector resolves a selector (JSON Pointer tokens with '*' wildcard and
// array indices) into the set of concrete token paths it addresses.
func expandSelector(root any, selector string) ([][]string, error) {
	tokens, err := splitPointer(selector)
	if err != nil {
		return nil, overlayInvalidAt(err.Error(), 1, 1)
	}
	paths := [][]string{{}}
	for _, token := range tokens {
		next := make([][]string, 0, len(paths))
		for _, prefix := range paths {
			value, ok := getNode(root, prefix)
			if !ok {
				continue
			}
			if token == "*" {
				switch typed := value.(type) {
				case map[string]any:
					for _, key := range sortedKeys(typed) {
						next = append(next, appendPath(prefix, key))
					}
				case []any:
					for index := range typed {
						next = append(next, appendPath(prefix, strconv.Itoa(index)))
					}
				}
				continue
			}
			switch typed := value.(type) {
			case map[string]any:
				if _, exists := typed[token]; exists {
					next = append(next, appendPath(prefix, token))
				}
			case []any:
				index, convErr := strconv.Atoi(token)
				if convErr != nil {
					return nil, overlayInvalidAt(fmt.Sprintf("array index %q is not a number", token), 1, 1)
				}
				if index >= 0 && index < len(typed) {
					next = append(next, appendPath(prefix, token))
				}
			}
		}
		paths = next
		if len(paths) == 0 {
			break
		}
	}
	return paths, nil
}

func appendPath(prefix []string, token string) []string {
	result := make([]string, len(prefix)+1)
	copy(result, prefix)
	result[len(prefix)] = token
	return result
}

// getNode reads the value at a concrete token path.
func getNode(root any, path []string) (any, bool) {
	current := root
	for _, token := range path {
		switch typed := current.(type) {
		case map[string]any:
			value, ok := typed[token]
			if !ok {
				return nil, false
			}
			current = value
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil || index < 0 || index >= len(typed) {
				return nil, false
			}
			current = typed[index]
		default:
			return nil, false
		}
	}
	return current, true
}

// setNode writes a value at a concrete token path, rebuilding array parents.
func setNode(root any, path []string, value any) (any, error) {
	if len(path) == 0 {
		return deepCopy(value), nil
	}
	return setNodeRecursive(root, path, deepCopy(value))
}

func setNodeRecursive(node any, path []string, value any) (any, error) {
	token := path[0]
	switch typed := node.(type) {
	case map[string]any:
		copy := make(map[string]any, len(typed))
		for key, child := range typed {
			copy[key] = child
		}
		if len(path) == 1 {
			copy[token] = value
			return copy, nil
		}
		child, ok := copy[token]
		if !ok {
			return nil, overlayInvalidAt(fmt.Sprintf("path %q does not exist", strings.Join(path, "/")), 1, 1)
		}
		updated, err := setNodeRecursive(child, path[1:], value)
		if err != nil {
			return nil, err
		}
		copy[token] = updated
		return copy, nil
	case []any:
		copy := make([]any, len(typed))
		copySlice := append(copy[:0], typed...)
		index, err := strconv.Atoi(token)
		if err != nil || index < 0 || index >= len(copySlice) {
			return nil, overlayInvalidAt(fmt.Sprintf("path %q does not exist", strings.Join(path, "/")), 1, 1)
		}
		if len(path) == 1 {
			copySlice[index] = value
			return copySlice, nil
		}
		updated, err := setNodeRecursive(copySlice[index], path[1:], value)
		if err != nil {
			return nil, err
		}
		copySlice[index] = updated
		return copySlice, nil
	default:
		return nil, overlayInvalidAt(fmt.Sprintf("path %q does not resolve to a container", strings.Join(path, "/")), 1, 1)
	}
}

// removeNode removes the value at a concrete token path, rebuilding parents.
// It returns the updated root and whether a value was removed.
func removeNode(root any, path []string) (any, bool, error) {
	if len(path) == 0 {
		return root, false, nil
	}
	updated, removed, err := removeNodeRecursive(root, path)
	return updated, removed, err
}

func removeNodeRecursive(node any, path []string) (any, bool, error) {
	token := path[0]
	switch typed := node.(type) {
	case map[string]any:
		if len(path) == 1 {
			if _, exists := typed[token]; !exists {
				return nil, false, overlayInvalidAt(fmt.Sprintf("path %q does not exist", token), 1, 1)
			}
			copy := make(map[string]any, len(typed))
			for key, child := range typed {
				if key != token {
					copy[key] = child
				}
			}
			return copy, true, nil
		}
		child, exists := typed[token]
		if !exists {
			return nil, false, overlayInvalidAt(fmt.Sprintf("path %q does not exist", strings.Join(path, "/")), 1, 1)
		}
		updated, removed, err := removeNodeRecursive(child, path[1:])
		if err != nil {
			return nil, false, err
		}
		copy := make(map[string]any, len(typed))
		for key, value := range typed {
			if key == token {
				copy[key] = updated
			} else {
				copy[key] = value
			}
		}
		return copy, removed, nil
	case []any:
		index, err := strconv.Atoi(token)
		if err != nil || index < 0 || index >= len(typed) {
			return nil, false, overlayInvalidAt(fmt.Sprintf("path %q does not exist", strings.Join(path, "/")), 1, 1)
		}
		if len(path) == 1 {
			updated := append([]any{}, typed[:index]...)
			updated = append(updated, typed[index+1:]...)
			return updated, true, nil
		}
		childUpdated, removed, err := removeNodeRecursive(typed[index], path[1:])
		if err != nil {
			return nil, false, err
		}
		updated := append([]any{}, typed...)
		updated[index] = childUpdated
		return updated, removed, nil
	default:
		return nil, false, overlayInvalidAt(fmt.Sprintf("path %q does not resolve to a container", strings.Join(path, "/")), 1, 1)
	}
}

// addNode inserts a value at a concrete path, supporting array indices and the
// '-' append marker.
func addNode(root any, path []string, value any) (any, error) {
	if len(path) == 0 {
		return deepCopy(value), nil
	}
	return addNodeRecursive(root, path, deepCopy(value))
}

func addNodeRecursive(node any, path []string, value any) (any, error) {
	token := path[0]
	switch typed := node.(type) {
	case map[string]any:
		copy := make(map[string]any, len(typed))
		for key, child := range typed {
			copy[key] = child
		}
		if len(path) == 1 {
			copy[token] = value
			return copy, nil
		}
		child, exists := copy[token]
		if !exists {
			child = map[string]any{}
		}
		updated, err := addNodeRecursive(child, path[1:], value)
		if err != nil {
			return nil, err
		}
		copy[token] = updated
		return copy, nil
	case []any:
		copySlice := append([]any{}, typed...)
		if token == "-" {
			copySlice = append(copySlice, value)
			if len(path) > 1 {
				return nil, overlayInvalidAt("array append marker '-' must be the final token", 1, 1)
			}
			return copySlice, nil
		}
		index, err := strconv.Atoi(token)
		if err != nil || index < 0 || index > len(copySlice) {
			return nil, overlayInvalidAt(fmt.Sprintf("path %q is not a valid array index", strings.Join(path, "/")), 1, 1)
		}
		if len(path) == 1 {
			copySlice = append(copySlice, nil)
			copy(copySlice[index+1:], copySlice[index:])
			copySlice[index] = value
			return copySlice, nil
		}
		childUpdated, err := addNodeRecursive(copySlice[index], path[1:], value)
		if err != nil {
			return nil, err
		}
		copySlice[index] = childUpdated
		return copySlice, nil
	default:
		return nil, overlayInvalidAt(fmt.Sprintf("path %q does not resolve to a container", strings.Join(path, "/")), 1, 1)
	}
}

// applyPatchOperation applies one RFC 6902 operation against the document.
func applyPatchOperation(document map[string]any, op patchOp) error {
	switch op.Op {
	case "add":
		path, err := splitPointer(op.Path)
		if err != nil {
			return overlayInvalidAt(err.Error(), op.Line, op.Column)
		}
		updated, addErr := addNode(document, path, deepCopy(op.Value))
		if addErr != nil {
			return addErr
		}
		replaceRoot(document, updated)
		return nil
	case "remove":
		path, err := splitPointer(op.Path)
		if err != nil {
			return overlayInvalidAt(err.Error(), op.Line, op.Column)
		}
		updated, removed, removeErr := removeNode(document, path)
		if removeErr != nil {
			return removeErr
		}
		if !removed {
			return overlayInvalidAt(fmt.Sprintf("path %q does not exist", op.Path), op.Line, op.Column)
		}
		replaceRoot(document, updated)
		return nil
	case "replace":
		path, err := splitPointer(op.Path)
		if err != nil {
			return overlayInvalidAt(err.Error(), op.Line, op.Column)
		}
		if _, exists := getNode(document, path); !exists {
			return overlayInvalidAt(fmt.Sprintf("path %q does not exist", op.Path), op.Line, op.Column)
		}
		updated, setErr := setNode(document, path, deepCopy(op.Value))
		if setErr != nil {
			return setErr
		}
		replaceRoot(document, updated)
		return nil
	case "move":
		return applyMoveCopy(document, op, true)
	case "copy":
		return applyMoveCopy(document, op, false)
	case "test":
		path, err := splitPointer(op.Path)
		if err != nil {
			return overlayInvalidAt(err.Error(), op.Line, op.Column)
		}
		value, exists := getNode(document, path)
		if !exists || !jsonEqual(value, op.Value) {
			return overlayInvalidAt(fmt.Sprintf("test failed at %q", op.Path), op.Line, op.Column)
		}
		return nil
	default:
		return overlayInvalidAt(fmt.Sprintf("unsupported patch operation %q", op.Op), op.Line, op.Column)
	}
}

func applyMoveCopy(document map[string]any, op patchOp, removeSource bool) error {
	fromPath, err := splitPointer(op.From)
	if err != nil {
		return overlayInvalidAt(err.Error(), op.Line, op.Column)
	}
	toPath, err := splitPointer(op.Path)
	if err != nil {
		return overlayInvalidAt(err.Error(), op.Line, op.Column)
	}
	value, exists := getNode(document, fromPath)
	if !exists {
		return overlayInvalidAt(fmt.Sprintf("from path %q does not exist", op.From), op.Line, op.Column)
	}
	value = deepCopy(value)
	if removeSource {
		removed, _, err := removeNode(document, fromPath)
		if err != nil {
			return err
		}
		replaceRoot(document, removed)
	}
	added, addErr := addNode(document, toPath, value)
	if addErr != nil {
		return addErr
	}
	replaceRoot(document, added)
	return nil
}

func replaceRoot(document map[string]any, updated any) {
	// The document argument is always a map[string]any; swap its contents.
	for key := range document {
		delete(document, key)
	}
	replacement, ok := updated.(map[string]any)
	if !ok {
		return
	}
	for key, value := range replacement {
		document[key] = value
	}
}

func sortedKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func splitPointer(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("pointer %q must start with '/'", pointer)
	}
	raw := strings.Split(pointer[1:], "/")
	tokens := make([]string, len(raw))
	for index, token := range raw {
		token = strings.ReplaceAll(token, "~1", "/")
		token = strings.ReplaceAll(token, "~0", "~")
		tokens[index] = token
	}
	return tokens, nil
}

func rejectDuplicateOverlayKeys(node *yaml.Node, prefix string) error {
	switch node.Kind {
	case yaml.MappingNode:
		seen := make(map[string]int, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			keyNode := node.Content[index]
			valueNode := node.Content[index+1]
			if firstLine, duplicate := seen[keyNode.Value]; duplicate {
				return overlayInvalidAt(fmt.Sprintf("duplicate key %q (first declared at line %d)", keyNode.Value, firstLine), keyNode.Line, keyNode.Column)
			}
			seen[keyNode.Value] = keyNode.Line
			if err := rejectDuplicateOverlayKeys(valueNode, prefix+"/"+keyNode.Value); err != nil {
				return err
			}
		}
	case yaml.SequenceNode:
		for index, child := range node.Content {
			if err := rejectDuplicateOverlayKeys(child, prefix+"/"+strconv.Itoa(index)); err != nil {
				return err
			}
		}
	}
	return nil
}

func overlayInvalidAt(message string, line, column int) error {
	return &OverlayInvalidError{Errors: []OverlayIssue{{Severity: "error", Code: overlayInvalidCode, Message: message, Line: line, Column: column}}}
}

func jsonEqual(left, right any) bool {
	leftBytes, leftErr := json.Marshal(left)
	rightBytes, rightErr := json.Marshal(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	return string(leftBytes) == string(rightBytes)
}
