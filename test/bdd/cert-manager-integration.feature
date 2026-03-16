Feature: Cert-Manager Integration for AxonOpsServer TLS

  As a cluster operator
  I want the AxonOpsServer controller to automatically manage TLS certificates using cert-manager
  So that I don't need to manually provision and rotate TLS certificates

  Background:
    Given the AxonOpsServer controller is deployed
    And cert-manager v1.13.0 or higher is installed in the cluster
    And I have a test namespace "axonops-test"

  Scenario: Create Certificate resources and reference default ClusterIssuer
    Given cert-manager is installed with version "v1.13.0" or higher
    When I create an AxonOpsServer CR with TLS enabled
    Then the controller should create a Certificate resource for the AxonOpsServer TLS
    And the Certificate resource should reference a ClusterIssuer named "axonops-selfsigned"
    And the TLS Secret should be created by cert-manager (with "cert-manager.io/certificate-name" annotation)

  Scenario: Create or update default ClusterIssuer for AxonOpsServer
    Given cert-manager is installed
    When I create an AxonOpsServer CR with TLS enabled
    Then the controller should ensure a ClusterIssuer exists for AxonOpsServer TLS
    And the ClusterIssuer should be named "axonops-selfsigned" (or configurable via status/annotation)
    And the ClusterIssuer should use SelfSigned provider (or CA if provided via secret reference)
    And the ClusterIssuer should NOT be deleted when the AxonOpsServer CR is deleted (shared resource)

  Scenario: Use custom ClusterIssuer if specified in AxonOpsServer spec
    Given cert-manager is installed
    And a custom ClusterIssuer named "my-custom-issuer" exists
    When I create an AxonOpsServer CR with spec.tls.issuer.name="my-custom-issuer"
    Then the controller should use "my-custom-issuer" instead of creating a default one
    And the Certificate resource should reference "my-custom-issuer"

  Scenario: TLS certificate renewal and rotation lifecycle
    Given cert-manager is installed
    And an AxonOpsServer CR with TLS enabled exists
    And a Certificate resource has been created for it
    When the certificate is within 30 days of expiration
    Then cert-manager should automatically request a renewal
    And the TLS Secret should be updated with the new certificate
    And the AxonOpsServer components should pick up the updated certificate without manual restart

  Scenario: Handle ClusterIssuer creation failure
    Given cert-manager is installed
    But ClusterIssuer creation fails (e.g., due to permission issues)
    When I create an AxonOpsServer CR with TLS enabled
    Then the controller should emit an error condition on the AxonOpsServer status
    And the AxonOpsServer should be in a "FailedToCreateIssuer" or "TLSConfigError" condition
    And the controller should log the root cause
    And the reconciliation should requeue for retry

  Scenario: Certificate resource ownership and garbage collection
    Given cert-manager is installed
    And an AxonOpsServer CR exists with TLS enabled
    When I delete the AxonOpsServer CR
    Then the Certificate resource should be deleted (owned by AxonOpsServer)
    And the ClusterIssuer should NOT be deleted (shared resource, no ownership)
    And the TLS Secret should be deleted (owned by AxonOpsServer)

  Scenario: Fail if cert-manager CRDs are not available
    Given cert-manager CRDs are not installed in the cluster
    When I create an AxonOpsServer CR with TLS enabled
    Then the controller should emit an error condition on the AxonOpsServer status
    And the condition should indicate "Cert-manager not found" or "Certificate CRDs unavailable"
    And the reconciliation should requeue for retry

  Scenario: Support custom CA certificate for ClusterIssuer
    Given cert-manager is installed
    And a Secret named "ca-key-pair" with CA cert and key exists in the controller namespace
    When I create an AxonOpsServer CR with spec.tls.issuer.kind="ClusterIssuer" and spec.tls.issuer.caSecretRef
    Then the controller should create a CA-based ClusterIssuer using the provided CA secret
    And Certificates signed by this issuer should include the CA chain in the TLS Secret
