# Security Policy

## Supported Versions

| Version           | Supported          |
|-------------------|--------------------|
| Latest v0.x minor | :white_check_mark: |
| Older releases    | :x:                |

Only the latest minor release receives security fixes.

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Report vulnerabilities privately through [axonops.com/contact](https://axonops.com/contact).
Include a description of the issue and its impact, steps to reproduce, and the
affected operator version and CRDs.

We acknowledge reports within 48 hours and triage them within 7 days. We follow
coordinated disclosure: details are not published until a fixed release is
available.

## Verifying Release Artifacts

Releases after v0.1.0 are signed with [Sigstore cosign](https://docs.sigstore.dev/)
keyless signing from the `release.yml` GitHub Actions workflow. Every release
ships with an SBOM and SLSA build provenance.

| Artifact | Location | Signature | SBOM | Provenance |
|----------|----------|-----------|------|------------|
| Container image | `ghcr.io/axonops/axonops-operator` | cosign (registry) | BuildKit SPDX attestation | BuildKit + GitHub attestation |
| Helm chart | `oci://ghcr.io/axonops/charts/axonops-operator` | cosign (registry) | — | GitHub attestation |
| `install.yaml`, `axonops-export-*` | GitHub release | `<file>.sigstore.json` bundle | `axonops-operator.spdx.json` | GitHub attestation |

The examples below use version `0.2.0`. Replace it with the version you install.
They need [cosign](https://docs.sigstore.dev/cosign/system_config/installation/) v3
or later and, for attestations, the [GitHub CLI](https://cli.github.com/).

All signatures are bound to this certificate identity and issuer:

```bash
export IDENTITY='^https://github\.com/axonops/axonops-operator/\.github/workflows/release\.yml@refs/tags/v'
export ISSUER='https://token.actions.githubusercontent.com'
```

### Container image

```bash
cosign verify ghcr.io/axonops/axonops-operator:0.2.0 \
  --certificate-identity-regexp "$IDENTITY" \
  --certificate-oidc-issuer "$ISSUER"

gh attestation verify oci://ghcr.io/axonops/axonops-operator:0.2.0 \
  --repo axonops/axonops-operator
```

Inspect the SBOM attached to the image:

```bash
docker buildx imagetools inspect ghcr.io/axonops/axonops-operator:0.2.0 \
  --format '{{ json .SBOM }}'
```

### Helm chart

```bash
cosign verify ghcr.io/axonops/charts/axonops-operator:0.2.0 \
  --certificate-identity-regexp "$IDENTITY" \
  --certificate-oidc-issuer "$ISSUER"

gh attestation verify oci://ghcr.io/axonops/charts/axonops-operator:0.2.0 \
  --repo axonops/axonops-operator
```

### Release files

Download the file, `checksums.txt` and `checksums.txt.sigstore.json` from the
[releases page](https://github.com/axonops/axonops-operator/releases), then:

```bash
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp "$IDENTITY" \
  --certificate-oidc-issuer "$ISSUER"

sha256sum --check --ignore-missing checksums.txt

gh attestation verify axonops-export-linux-amd64 \
  --repo axonops/axonops-operator
```

Each file also has its own `<file>.sigstore.json` bundle, which you can check
with `cosign verify-blob` in the same way.

## Security Scanning

The `Security` workflow runs on every pull request, on pushes to `main`, and
weekly. It runs `govulncheck`, CodeQL, and a Trivy scan of the operator image
that fails on fixable `HIGH` or `CRITICAL` findings. The `OpenSSF Scorecard`
workflow runs on pushes to `main` and weekly. Results appear in the repository's
**Security → Code scanning** tab.

A release is published only when the tag points at a commit on `main` and both
`make lint` and `make test` pass.

---

Maintained by [AxonOps](https://axonops.com). Support at [axonops.com/contact](https://axonops.com/contact).
