package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
)

// maxErrorMessageRunes bounds the server message echoed into a diagnostic.
const maxErrorMessageRunes = 300

// tokenPattern matches Ramble API tokens so one that a server echoes back in
// a message never reaches a diagnostic.
var tokenPattern = regexp.MustCompile(`rmbl_[A-Za-z0-9_-]+`)

// apiErrorDiag turns a failed Ramble response into a diagnostic. It decodes
// the /v1 error shape {"error":{"code","message"}} and falls back to the
// status text when the body is not that shape. The raw body and request
// headers are never echoed.
func apiErrorDiag(summary string, status int, body []byte) diag.Diagnostic {
	code, message := decodeAPIError(body)
	message = tokenPattern.ReplaceAllString(message, "[redacted]")
	if r := []rune(message); len(r) > maxErrorMessageRunes {
		message = string(r[:maxErrorMessageRunes]) + "..."
	}

	detail := fmt.Sprintf("Ramble answered %d", status)
	switch {
	case code != "" && message != "":
		detail += " " + code + ": " + message
	case code != "":
		detail += " " + code
	case message != "":
		detail += ": " + message
	default:
		detail += " " + http.StatusText(status)
	}
	hint := codeHint(code)
	if hint == "" {
		hint = statusHint(status)
	}
	if hint != "" {
		detail += "\n\n" + hint
	}
	return diag.NewErrorDiagnostic(summary, detail)
}

func decodeAPIError(body []byte) (code, message string) {
	var e struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) != nil || e.Error == nil {
		return "", ""
	}
	return e.Error.Code, e.Error.Message
}

// codeHint returns advice for the error codes Ramble gives for organisations.
// It takes precedence over the hint for the status.
func codeHint(code string) string {
	switch code {
	case "org_not_empty":
		return "Delete or move the organisation's packs and jobs first. When they are in this configuration, set " +
			"ramble_resource.namespace = ramble_organization.<name>.name so that Terraform destroys them before the organisation."
	case "last_owner":
		return "An organisation must always keep an owner: make another member owner first, or destroy the organisation itself."
	case "invalid_role":
		return "The role must be owner or member."
	case "invalid_name":
		return "An organisation name is 1-64 characters of letters, digits, dot, dash and underscore, starting with a letter or digit, " +
			"and some names are reserved."
	}
	return ""
}

func statusHint(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "The API token was rejected: it may be invalid, expired or revoked. Check the token attribute of the provider or the " + envToken + " environment variable."
	case http.StatusForbidden:
		return "The token needs write scope. A token restricted to one organisation cannot change another namespace, only an owner may change or delete an organisation, " +
			"and only a member may list its members."
	case http.StatusConflict:
		return "The object already exists or is in a conflicting state. To manage an existing object with Terraform, import it: " +
			"the documentation of the resource gives the import id."
	case http.StatusTooManyRequests:
		return "Ramble is applying a rate limit to this token. Wait a moment and apply again."
	}
	return ""
}

// isDuplicateVersion reports whether a failed add-version response means the
// version already exists, as opposed to any other conflict.
func isDuplicateVersion(status int, body []byte) bool {
	if status != http.StatusConflict {
		return false
	}
	_, message := decodeAPIError(body)
	return strings.Contains(message, "already exists")
}
