# External Secrets Operator custom resource fails to open

- Submitted: 2026-07-29
- Priority: high
- Area: resource view / CRDs

Trying to open the `ExternalSecret` custom resource (from the
external-secrets-operator CRDs) errors out instead of showing its detail
view. Other CRDs presumably need checking too, but this one reproduces
reliably. Need to find the actual error (likely a schema/decoding edge case
specific to that CRD) and fix the resource viewer to handle it instead of
erroring.
