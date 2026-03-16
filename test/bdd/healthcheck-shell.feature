Feature: AxonOpsHealthcheckShell lifecycle for custom script monitoring
  As an operator user
  I want to define custom shell script healthchecks using AxonOpsHealthcheckShell CRs
  That run arbitrary scripts to verify system health and generate alerts
  So that I can implement complex health verification logic beyond standard protocols

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And the AxonOps CRDs are registered
    And a valid AxonOpsConnection CR exists

  Scenario: Create shell healthcheck with bash script
    Given an AxonOpsHealthcheckShell CR with:
      - spec.connectionRef set to a valid AxonOpsConnection
      - spec.script set to "#!/bin/bash\ndu -sh /var/lib/cassandra | awk '{print $1}' | grep -E '^[0-9]+G'"
      - spec.shell set to "/bin/bash"
    When the operator reconciles the AxonOpsHealthcheckShell CR
    Then the AxonOps API should be called to create a shell healthcheck
    And the AxonOpsHealthcheckShell status should have field "syncedCheckID"
    And the status condition should show "Ready=True"

  Scenario: Update shell script content
    Given an existing AxonOpsHealthcheckShell CR with syncedCheckID in status
    When the user updates the CR to change spec.script to a different verification command
    And the operator reconciles the AxonOpsHealthcheckShell CR
    Then the AxonOps API should be called with HTTP PUT to update the script
    And the syncedCheckID should remain unchanged
    And the new script should be used for subsequent healthcheck runs

  Scenario: Delete shell healthcheck
    Given an existing AxonOpsHealthcheckShell CR with syncedCheckID in status
    When the user deletes the AxonOpsHealthcheckShell CR
    Then the operator should call the AxonOps API to delete the healthcheck
    And the finalizer should be removed
    And the CR should be deleted from Kubernetes

  Scenario: Default shell is /bin/sh when omitted
    Given an AxonOpsHealthcheckShell CR with:
      - spec.script set to "test -f /etc/config && echo ok || echo error"
      - spec.shell is omitted
    When the operator reconciles the AxonOpsHealthcheckShell CR
    Then the operator should default spec.shell to "/bin/sh"
    And the AxonOps API should be called with shell "/bin/sh"
    And the healthcheck should be created successfully

  Scenario: Readonly flag prevents execution on production clusters
    Given an AxonOpsHealthcheckShell CR with:
      - spec.script set to "rm -rf /tmp/*"
      - spec.readonly set to true
    When the operator reconciles the AxonOpsHealthcheckShell CR
    Then the operator should flag this healthcheck as read-only in the AxonOps API
    And the destructive script should not be executable on production environments
    And the AxonOpsHealthcheckShell status condition should show "Ready=True"

  Scenario: Missing AxonOpsConnection reference
    Given an AxonOpsHealthcheckShell CR with spec.connectionRef set to "nonexistent"
    When the operator reconciles the AxonOpsHealthcheckShell CR
    Then the status condition should show "Ready=False"
    And the condition reason should be "ConnectionNotFound"
    And the healthcheck should not be synced to the AxonOps API

  Scenario: Multiple shell healthchecks with different scripts
    Given three AxonOpsHealthcheckShell CRs with different verification scripts:
      - Disk space check
      - Memory usage check
      - Service port availability check
    When the operator reconciles all healthchecks
    Then each shell healthcheck should be created independently
    And all should have syncedCheckID in their status
    And all should show "Ready=True"
    And each script should be synced separately to the AxonOps API
