package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
)

var tagPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

// tagValidator checks one tag against the form Ramble stores: lowercase,
// 2-20 characters of a-z, 0-9 and dash. Ramble never normalises tags and
// answers 422 invalid_tags otherwise, so the plan fails early instead.
type tagValidator struct{}

func (tagValidator) Description(context.Context) string {
	return "tag must be 2-20 characters of a-z, 0-9 and -"
}

func (v tagValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (tagValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	t := req.ConfigValue.ValueString()
	switch {
	case len(t) < 2 || len(t) > 20:
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid tag",
			fmt.Sprintf("Tag %q must be 2-20 characters.", t))
	case !tagPattern.MatchString(t):
		if l := strings.ToLower(t); l != t && tagPattern.MatchString(l) {
			resp.Diagnostics.AddAttributeError(req.Path, "Invalid tag",
				fmt.Sprintf("Tag %q must be lowercase: use %q.", t, l))
			return
		}
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid tag",
			fmt.Sprintf("Tag %q may contain only a-z, 0-9 and -.", t))
	}
}

// versionValidator checks a version string the way Ramble stores it: 1-64
// characters, trimmed. Ramble trims whitespace, so a padded version would
// never match the value read back.
type versionValidator struct{}

func (versionValidator) Description(context.Context) string {
	return "version must be 1-64 bytes without leading or trailing whitespace"
}

func (v versionValidator) MarkdownDescription(ctx context.Context) string { return v.Description(ctx) }

func (versionValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	s := req.ConfigValue.ValueString()
	// Ramble limits the version to 64 bytes, not characters.
	if n := len(s); n < 1 || n > 64 {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid version",
			"The version must be 1-64 bytes long.")
		return
	}
	if strings.TrimFunc(s, unicode.IsSpace) != s {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid version",
			"The version must not have leading or trailing whitespace.")
	}
}
