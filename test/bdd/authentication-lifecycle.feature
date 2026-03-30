Feature: Manage database authentication credentials lifecycle
  As an operator user
  I want flexible credential management for TimeSeries and Search databases
  Supporting SecretRef, inline credentials, and auto-generated passwords
  So that I can choose the credential management strategy that fits my security posture

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And the AxonOps CRDs are registered

  Scenario: Use existing Secret via secretRef
    Given an existing Secret named "db-creds" with keys "username" and "password"
    And an AxonOpsPlatform CR with spec.timeSeries.authentication.secretRef set to "db-creds"
    When the operator reconciles the AxonOpsPlatform CR
    Then the axondb-timeseries StatefulSet should be configured with credentials from "db-creds"
    And no new Secret should be created for TimeSeries auth
    And the AxonOpsPlatform status should have condition "TimeSeriesReady=True"

  Scenario: Use inline username and password
    Given an AxonOpsPlatform CR with:
      - spec.timeSeries.authentication.username set to "cassandra"
      - spec.timeSeries.authentication.password set to "MySecurePass123!"
    When the operator reconciles the AxonOpsPlatform CR
    Then a managed Secret should be created containing the provided credentials
    And the axondb-timeseries StatefulSet should reference this managed Secret
    And the AxonOpsPlatform status should have condition "TimeSeriesReady=True"

  Scenario: Auto-generate credentials when none provided
    Given an AxonOpsPlatform CR with spec.timeSeries.authentication fully omitted
    When the operator reconciles the AxonOpsPlatform CR
    Then a managed Secret should be created with auto-generated credentials
    And the password should meet complexity requirements (uppercase, digit, special char)
    And the axondb-timeseries StatefulSet should reference the auto-generated Secret
    And the username should default to "cassandra"

  Scenario: Auto-generated password persists across reconcile loops
    Given an AxonOpsPlatform CR with auto-generated TimeSeries credentials
    And the managed Secret containing the auto-generated password
    When the operator reconciles the AxonOpsPlatform CR again
    Then the same managed Secret should be referenced (no regeneration)
    And the password should remain unchanged
    And the StatefulSet should not be updated

  Scenario: SecretRef takes priority over inline credentials
    Given an AxonOpsPlatform CR with:
      - spec.timeSeries.authentication.secretRef set to "existing-secret"
      - spec.timeSeries.authentication.username and password also set
    When the operator reconciles the AxonOpsPlatform CR
    Then the credentials from "existing-secret" should be used
    And the inline username/password should be ignored
    And a condition should note that secretRef takes priority

  Scenario: Referenced Secret does not exist
    Given an AxonOpsPlatform CR with spec.timeSeries.authentication.secretRef set to "nonexistent"
    And no Secret named "nonexistent" exists
    When the operator reconciles the AxonOpsPlatform CR
    Then the AxonOpsPlatform status should have condition "TimeSeriesReady=False"
    And the condition reason should be "SecretNotFound"
    And the condition message should indicate the missing Secret name
    And reconciliation should not proceed for the StatefulSet

  Scenario: Referenced Secret exists but is missing required key
    Given an existing Secret named "incomplete-creds" with only the "username" key
    And an AxonOpsPlatform CR with spec.timeSeries.authentication.secretRef set to "incomplete-creds"
    When the operator reconciles the AxonOpsPlatform CR
    Then the AxonOpsPlatform status should have condition "TimeSeriesReady=False"
    And the condition reason should be "SecretKeyMissing"
    And the condition message should indicate which key is missing (e.g., "password")

  Scenario: Update credentials from SecretRef to inline
    Given an existing AxonOpsPlatform CR using spec.timeSeries.authentication.secretRef
    And the axondb-timeseries StatefulSet is running
    When the user updates the CR to use spec.timeSeries.authentication.username and password instead
    Then a managed Secret should be created with the inline credentials
    And the StatefulSet should be updated to reference the new Secret
    And the AxonOpsPlatform status should transition to "Ready=True"
    And a condition should document this credential source change
