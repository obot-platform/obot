// Package auditlog converts persisted audit rows into the normalized public read model.
//
// Authorization, decryption, and sensitive-field blanking remain the responsibility of the
// gateway and API layers. Keeping those concerns outside this package makes presentation and
// outcome classification deterministic and reusable by HTTP responses and JSONL exports.
package auditlog
