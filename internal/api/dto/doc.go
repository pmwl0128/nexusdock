// Package dto marks the Nexus HTTP DTO boundary.
//
// Public DTOs are generated from contracts/openapi/nexus.yaml into
// generated/nexuscontracts. This package intentionally contains no handwritten
// transport structs; business modules must consume the generated package so the
// contracts directory remains the single source of truth.
package dto
