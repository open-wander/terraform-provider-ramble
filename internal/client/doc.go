// Package rambleapi is the Go client for the Ramble /v1 API, generated from
// openapi.yaml, which is a verbatim copy of the Ramble server's
// api-docs/openapi.yaml. Build clients with New, never with the generated
// constructors, so a token always travels over the hardened HTTP client.
package rambleapi

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.9.0 -generate types,client -package rambleapi -o client.gen.go openapi.yaml
