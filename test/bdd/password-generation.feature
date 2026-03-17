Feature: Auto-generated password complexity and persistence
  As a Kubernetes operator
  I need to generate secure passwords that meet complexity requirements
  So that database credentials are strong and consistent across reconciliations

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And a namespace "axonops-test" exists

  Scenario: Auto-generated password meets complexity requirements
    Given an AxonOpsServer CR with no credentials specified for TimeSeries
    When the operator reconciles and auto-generates a password
    Then the generated password should be 32 characters long
    And the password should contain at least one uppercase letter
    And the password should contain at least one digit
    And the password should contain at least one special character

  Scenario: Auto-generated password is stored in a managed Secret
    Given an AxonOpsServer CR with no credentials specified for TimeSeries
    When the operator reconciles and auto-generates credentials
    Then a Secret named "{server-name}-timeseries-auth" should be created
    And the Secret should have key "AXONOPS_DB_USER"
    And the Secret should have key "AXONOPS_DB_PASSWORD"
    And the Secret should have an owner reference to the AxonOpsServer CR

  Scenario: Auto-generated password persists across reconcile loops
    Given an AxonOpsServer CR with auto-generated TimeSeries credentials
    And the auth Secret already exists with a generated password
    When the operator reconciles the AxonOpsServer CR again
    Then the existing password in the Secret should not be changed
    And the AXONOPS_DB_USER value should remain the same

  Scenario: Keystore password is generated for TLS components
    Given an AxonOpsServer CR with internal TimeSeries and TLS enabled
    When the operator reconciles the AxonOpsServer CR
    Then a keystore password Secret should be created for TimeSeries
    And the keystore password should be 24 characters long

  Scenario: Search auto-generated credentials use correct keys
    Given an AxonOpsServer CR with no credentials specified for Search
    When the operator reconciles and auto-generates credentials
    Then a Secret named "{server-name}-search-auth" should be created
    And the Secret should have key "AXONOPS_SEARCH_USER"
    And the Secret should have key "AXONOPS_SEARCH_PASSWORD"
