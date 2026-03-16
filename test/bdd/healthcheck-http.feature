Feature: AxonOpsHealthcheckHTTP lifecycle for HTTP endpoint monitoring
  As an operator user
  I want to define HTTP healthcheck monitors using AxonOpsHealthcheckHTTP CRs
  That verify external and internal endpoints are responding correctly
  So that I can automatically detect service degradation and outages

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And the AxonOps CRDs are registered
    And a valid AxonOpsConnection CR exists

  Scenario: Create HTTP healthcheck with GET request
    Given an AxonOpsHealthcheckHTTP CR with:
      - spec.connectionRef set to a valid AxonOpsConnection
      - spec.url set to "https://api.example.com/health"
      - spec.method set to "GET"
      - spec.expectedStatus set to 200
    When the operator reconciles the AxonOpsHealthcheckHTTP CR
    Then the AxonOps API should be called to create an HTTP healthcheck
    And the AxonOpsHealthcheckHTTP status should have field "syncedCheckID"
    And the status condition should show "Ready=True"

  Scenario: Create healthcheck with custom headers and body
    Given an AxonOpsHealthcheckHTTP CR with:
      - spec.url set to "https://api.example.com/webhook"
      - spec.method set to "POST"
      - spec.headers set to {"Authorization": "Bearer token123", "Content-Type": "application/json"}
      - spec.body set to "{\"test\": true}"
      - spec.expectedStatus set to 202
    When the operator reconciles the AxonOpsHealthcheckHTTP CR
    Then the AxonOps API should be called with the custom headers and body
    And the syncedCheckID should be stored in status
    And the status condition should show "Ready=True"

  Scenario: Update healthcheck interval
    Given an existing AxonOpsHealthcheckHTTP CR with syncedCheckID in status
    When the user updates the CR to change spec.interval from "60" to "30"
    And the operator reconciles the AxonOpsHealthcheckHTTP CR
    Then the AxonOps API should be called with HTTP PUT to update the interval
    And the syncedCheckID should remain unchanged
    And the healthcheck should run more frequently

  Scenario: Delete healthcheck
    Given an existing AxonOpsHealthcheckHTTP CR with syncedCheckID in status
    When the user deletes the AxonOpsHealthcheckHTTP CR
    Then the operator should call the AxonOps API to delete the healthcheck
    And the finalizer should be removed
    And the CR should be deleted from Kubernetes

  Scenario: TLS skip verify for self-signed certificates
    Given an AxonOpsHealthcheckHTTP CR with:
      - spec.url set to "https://internal.service.com/health"
      - spec.tlsSkipVerify set to true
    When the operator reconciles the AxonOpsHealthcheckHTTP CR
    Then the AxonOps API should be called with tlsSkipVerify flag set
    And the healthcheck should succeed even with invalid or self-signed TLS certificates
    And the status condition should show "Ready=True"

  Scenario: Default method is GET when omitted
    Given an AxonOpsHealthcheckHTTP CR with:
      - spec.url set to "https://api.example.com/status"
      - spec.method is omitted
    When the operator reconciles the AxonOpsHealthcheckHTTP CR
    Then the operator should default spec.method to "GET"
    And the AxonOps API should be called with method "GET"
    And the healthcheck should be created successfully

  Scenario: Default expectedStatus is 200 when omitted
    Given an AxonOpsHealthcheckHTTP CR with:
      - spec.url set to "https://api.example.com/ok"
      - spec.expectedStatus is omitted
    When the operator reconciles the AxonOpsHealthcheckHTTP CR
    Then the operator should default spec.expectedStatus to 200
    And the AxonOps API should be called with expectedStatus 200
    And the healthcheck should pass only on HTTP 200 responses
