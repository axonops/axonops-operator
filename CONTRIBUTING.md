# Contributing to AxonOps Operator

Thank you for your interest in contributing! This guide explains our development workflow and how to contribute.

## Feature Development Workflow

### 1. Create GitHub Issue

Start by creating a GitHub issue describing what you want to build:

- **Title**: Descriptive summary (e.g., "feat: add webhook support")
- **Description**: Problem statement, proposed solution, acceptance criteria
- **Labels**: Add appropriate labels (`feature`, `bug`, `enhancement`)

### 2. Planning Session

For significant features, design the implementation before writing code:

- Explore affected files and architecture
- Identify dependencies and risks
- Document the approach and get approval before coding

### 3. Create Feature Branch

Branch naming: `<type>/<issue-number>-<description>`

```bash
git checkout -b feature/42-webhook-support
```

**Branch types**:
- `feature/*` — New functionality
- `fix/*` — Bug fixes
- `docs/*` — Documentation
- `chore/*` — Dependency updates, tooling

### 4. Make Changes

Follow these guidelines:

**Code Quality**:
```bash
make fmt      # Format code
make vet      # Vet code
make lint     # Run linter
make test     # Run tests
```

**Commit Messages**:
```
feat: add webhook support for alerts

- Implement HTTPServer for webhook endpoint
- Add WebhookConfig CRD
- Support API key authentication

Closes #42
```

Use conventional commits:
- `feat:` for features
- `fix:` for bug fixes
- `docs:` for documentation
- `chore:` for tooling
- `refactor:` for refactoring

**Always include `Closes #<issue-number>` in commit message.**

### 5. Create Pull Request

```bash
git push origin feature/42-webhook-support
```

PR body should include:
- What the PR does
- Reference to issue: `Closes #42`
- Description of testing
- Any breaking changes

### 6. Review & Merge

- Ensure CI checks pass
- Wait for maintainer review
- Address feedback
- Merge to `main`
- GitHub issue closes automatically

---

## Local Development Setup

### Prerequisites

- Go 1.25+ (check `go.mod`)
- Kubernetes 1.28+ (for testing)
- Make
- kubebuilder (optional, for code generation)

### Install Dependencies

```bash
go mod download
```

### Run Tests

```bash
# Run all unit tests
make test

# Run specific test
go test ./internal/controller -run TestNamePattern

# With coverage
make test
```

### Run Locally

```bash
# Install CRDs into current cluster
make install

# Run controller locally
make run
```

### Build Binary

```bash
make build
```

### Code Generation

After modifying `*_types.go` files:

```bash
make manifests   # Regenerate CRDs and RBAC
make generate    # Regenerate deepcopy methods
```

---

## Code Guidelines

### Architecture

The operator uses kubebuilder with a multi-group layout:

```
api/
  ├── v1alpha1/              # core.axonops.com group
  │   ├── axonopsplatform_types.go
  │   └── axonopsconnection_types.go
  ├── alerts/v1alpha1/       # alerts.axonops.com group
  │   ├── axonopsmetricalert_types.go
  │   └── ...
  ├── backups/v1alpha1/      # backups.axonops.com group
  │   └── axonopsbackup_types.go
  └── kafka/v1alpha1/        # kafka.axonops.com group
      ├── axonopskafkatopic_types.go
      └── ...

internal/
  ├── controller/            # AxonOpsPlatform controller
  ├── controller/alerts/     # Alert controllers
  ├── controller/backups/    # Backup controllers
  ├── controller/kafka/      # Kafka controllers
  ├── controller/common/     # Shared helpers (connection resolver, SafeConditionMsg)
  └── axonops/               # AxonOps API client
```

### Writing Controllers

```go
// Always use structured logging
log := logf.FromContext(ctx)
log.Info("Reconciling resource", "resource", req.NamespacedName)

// Use finalizers for cleanup
if !controllerutil.ContainsFinalizer(obj, finalizerName) {
    controllerutil.AddFinalizer(obj, finalizerName)
}

// Set controller reference for ownership
if err := controllerutil.SetControllerReference(owner, obj, r.Scheme); err != nil {
    return err
}

// Use status conditions
meta.SetStatusCondition(&obj.Status.Conditions, metav1.Condition{
    Type:               "Ready",
    Status:             metav1.ConditionTrue,
    ObservedGeneration: obj.Generation,
    Reason:             "Synced",
    Message:            "Resource synced successfully",
})
```

### Testing

Write unit tests for all controllers:

```go
// Use envtest for realistic testing
It("should reconcile successfully", func() {
    // Create test resource
    alert := &alertsv1alpha1.AxonOpsMetricAlert{...}
    Expect(k8sClient.Create(ctx, alert)).To(Succeed())

    // Trigger reconciliation
    Eventually(...)

    // Verify status
    Expect(alert.Status.Ready).To(Equal(true))
})
```

### Documentation

- Add comments to exported functions/types
- Update CLAUDE.md for significant changes
- Keep examples in `examples/` up to date
- Document new CRD fields with kubebuilder markers

---

## Filing Issues

When reporting bugs:

1. **Check existing issues** — Avoid duplicates
2. **Use bug report template** — Provides structure
3. **Include environment details** — K8s version, operator version, etc.
4. **Provide reproducible steps** — Steps to reproduce the issue
5. **Include logs** — Error messages and operator logs

---

## Review Process

All PRs are reviewed by maintainers. We look for:

- ✅ Code quality and style
- ✅ Test coverage
- ✅ Documentation updates
- ✅ No breaking changes (or documented)
- ✅ Proper commit history
- ✅ CI checks passing

---

## Commit History

Commits should be logical, focused, and reversible. Avoid:

- ❌ "WIP" commits mixed with real changes
- ❌ Unrelated changes in one commit
- ❌ Commits that break tests

Good commits are self-contained and can be understood in isolation.

---

## Questions?

- Check CLAUDE.md for project context
- Review existing issues and PRs for examples
- Ask in GitHub discussions or issues

Happy contributing!
