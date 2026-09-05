package contracttest

import (
	"reflect"
	"strings"
	"testing"

	"github.com/meridian-labs/meridian/internal/generated/api"
)

func TestGeneratedGoPreservesOptionalAndNullableSemantics(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		owner reflect.Type
		field string
	}{
		{owner: reflect.TypeFor[api.TenantCreateRequest](), field: "Quota"},
		{owner: reflect.TypeFor[api.UserPatchRequest](), field: "DisplayName"},
	} {
		field, ok := test.owner.FieldByName(test.field)
		if !ok {
			t.Fatalf("generated %s.%s field is missing", test.owner.Name(), test.field)
		}
		if field.Type.Kind() != reflect.Pointer {
			t.Errorf("generated %s.%s type = %s, want pointer preserving optional or nullable semantics", test.owner.Name(), test.field, field.Type)
		}
	}

	for _, test := range []struct {
		owner reflect.Type
		field string
	}{
		{owner: reflect.TypeFor[api.UserCreateRequest](), field: "Email"},
		{owner: reflect.TypeFor[api.UserPatchRequest](), field: "Email"},
		{owner: reflect.TypeFor[api.User](), field: "Email"},
		{owner: reflect.TypeFor[api.ApiToken](), field: "ExpiresAt"},
	} {
		field, ok := test.owner.FieldByName(test.field)
		if !ok {
			t.Fatalf("generated %s.%s field is missing", test.owner.Name(), test.field)
		}
		if !strings.HasPrefix(field.Type.String(), "nullable.Nullable[") {
			t.Errorf("generated %s.%s type = %s, want nullable.Nullable preserving absent, null, and value states", test.owner.Name(), test.field, field.Type)
		}
	}
}
