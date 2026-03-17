Feature: Credential secret retention based on StorageClass reclaim policy
  As a cluster operator
  I want database credential secrets to survive AxonOpsServer CR deletion
    when the StorageClass retains PVCs
  So that retained PVCs remain usable when the CR is re-created

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And cert-manager is available in the cluster
    And no AxonOpsServer resources exist in the "default" namespace

  # --- StorageClass Retain: secrets survive CR deletion ---

  Scenario: Auth secrets survive CR deletion with Retain StorageClass
    Given a StorageClass "retain-sc" with reclaimPolicy "Retain"
    And an AxonOpsServer "my-axonops" with TimeSeries and Search using storageClassName "retain-sc"
    And the controller has generated auth secrets "my-axonops-timeseries-auth" and "my-axonops-search-auth"
    When I delete the AxonOpsServer "my-axonops"
    And the AxonOpsServer resource is fully removed
    Then the secret "my-axonops-timeseries-auth" should still exist in the namespace
    And the secret "my-axonops-search-auth" should still exist in the namespace
    And PVCs with prefix "data-my-axonops-timeseries-" should still exist
    And PVCs with prefix "data-my-axonops-search-" should still exist

  Scenario: Keystore password secrets survive CR deletion with Retain StorageClass
    Given a StorageClass "retain-sc" with reclaimPolicy "Retain"
    And an AxonOpsServer "my-axonops" with TimeSeries and Search using storageClassName "retain-sc"
    And the controller has generated keystore secrets "my-axonops-timeseries-keystore-password" and "my-axonops-search-keystore-password"
    When I delete the AxonOpsServer "my-axonops"
    And the AxonOpsServer resource is fully removed
    Then the secret "my-axonops-timeseries-keystore-password" should still exist in the namespace
    And the secret "my-axonops-search-keystore-password" should still exist in the namespace

  Scenario: Auth secrets have no owner reference with Retain StorageClass
    Given a StorageClass "retain-sc" with reclaimPolicy "Retain"
    And an AxonOpsServer "my-axonops" with TimeSeries using storageClassName "retain-sc"
    When the controller reconciles
    Then the secret "my-axonops-timeseries-auth" should not have an owner reference to AxonOpsServer

  # --- StorageClass Delete: secrets cascade-deleted ---

  Scenario: Auth secrets are cascade-deleted with Delete StorageClass
    Given a StorageClass "delete-sc" with reclaimPolicy "Delete"
    And an AxonOpsServer "my-axonops" with TimeSeries using storageClassName "delete-sc"
    And the controller has generated auth secret "my-axonops-timeseries-auth"
    Then the secret "my-axonops-timeseries-auth" should have an owner reference to AxonOpsServer
    When I delete the AxonOpsServer "my-axonops"
    And the AxonOpsServer resource is fully removed
    Then the secret "my-axonops-timeseries-auth" should not exist in the namespace

  Scenario: Secrets deleted via finalizer with Delete StorageClass
    Given a StorageClass "delete-sc" with reclaimPolicy "Delete"
    And an AxonOpsServer "my-axonops" with TimeSeries and Search using storageClassName "delete-sc"
    When I delete the AxonOpsServer "my-axonops"
    Then the finalizer should explicitly delete credential secrets for TimeSeries and Search

  # --- Re-creation reuses existing secrets ---

  Scenario: Re-created AxonOpsServer reuses existing auth secrets
    Given a StorageClass "retain-sc" with reclaimPolicy "Retain"
    And an AxonOpsServer "my-axonops" was previously deleted but its secrets and PVCs remain
    And the secret "my-axonops-timeseries-auth" contains key "AXONOPS_DB_PASSWORD" with value "old-password-123"
    When I create a new AxonOpsServer "my-axonops" with TimeSeries enabled using storageClassName "retain-sc"
    And the controller reconciles
    Then the secret "my-axonops-timeseries-auth" should contain key "AXONOPS_DB_PASSWORD" with value "old-password-123"
    And the TimeSeries StatefulSet pods should start successfully

  Scenario: Re-created AxonOpsServer reuses existing keystore password secrets
    Given a StorageClass "retain-sc" with reclaimPolicy "Retain"
    And an AxonOpsServer "my-axonops" was previously deleted but its secrets and PVCs remain
    And the secret "my-axonops-timeseries-keystore-password" exists with a previous password
    When I create a new AxonOpsServer "my-axonops" with TimeSeries enabled using storageClassName "retain-sc"
    And the controller reconciles
    Then the secret "my-axonops-timeseries-keystore-password" should retain its original password value

  # --- Default StorageClass behaviour ---

  Scenario: No explicit StorageClass uses cluster default
    Given a default StorageClass "standard" with reclaimPolicy "Delete"
    And an AxonOpsServer "my-axonops" with TimeSeries enabled and no storageClassName specified
    When the controller reconciles
    Then the secret "my-axonops-timeseries-auth" should have an owner reference to AxonOpsServer

  Scenario: Retain default StorageClass retains secrets
    Given a default StorageClass "premium" with reclaimPolicy "Retain"
    And an AxonOpsServer "my-axonops" with TimeSeries enabled and no storageClassName specified
    When the controller reconciles
    Then the secret "my-axonops-timeseries-auth" should not have an owner reference to AxonOpsServer

  Scenario: No StorageClass found defaults to Delete behaviour
    Given no StorageClasses exist in the cluster
    And an AxonOpsServer "my-axonops" with TimeSeries enabled
    When the controller reconciles
    Then the secret "my-axonops-timeseries-auth" should have an owner reference to AxonOpsServer

  # --- User-provided SecretRef is unaffected ---

  Scenario: User-provided SecretRef is never deleted by the operator
    Given a pre-existing secret "my-external-creds" with database credentials
    And an AxonOpsServer "my-axonops" with TimeSeries referencing SecretRef "my-external-creds"
    When I delete the AxonOpsServer "my-axonops"
    And the AxonOpsServer resource is fully removed
    Then the secret "my-external-creds" should still exist in the namespace
    And the secret "my-external-creds" should not have an owner reference to AxonOpsServer

  # --- Component disable transitions ---

  Scenario: Disabling a component with Retain StorageClass keeps secrets
    Given a StorageClass "retain-sc" with reclaimPolicy "Retain"
    And an AxonOpsServer "my-axonops" with TimeSeries enabled using storageClassName "retain-sc"
    And the controller has generated auth secret "my-axonops-timeseries-auth"
    When I update the AxonOpsServer "my-axonops" to disable TimeSeries
    And the controller reconciles
    Then the TimeSeries StatefulSet should be deleted
    But the secret "my-axonops-timeseries-auth" should still exist in the namespace
    And the secret "my-axonops-timeseries-keystore-password" should still exist in the namespace

  Scenario: Disabling a component with Delete StorageClass removes secrets
    Given a StorageClass "delete-sc" with reclaimPolicy "Delete"
    And an AxonOpsServer "my-axonops" with TimeSeries enabled using storageClassName "delete-sc"
    And the controller has generated auth secret "my-axonops-timeseries-auth"
    When I update the AxonOpsServer "my-axonops" to disable TimeSeries
    And the controller reconciles
    Then the TimeSeries StatefulSet should be deleted
    And the secret "my-axonops-timeseries-auth" should not exist in the namespace
    And the secret "my-axonops-timeseries-keystore-password" should not exist in the namespace

  # --- Secret labelling for discovery ---

  Scenario: Managed secrets are labelled for operator discovery
    Given an AxonOpsServer "my-axonops" with TimeSeries enabled
    When the controller creates the auth secret "my-axonops-timeseries-auth"
    Then the secret should have label "app.kubernetes.io/instance" set to "my-axonops"
    And the secret should have label "app.kubernetes.io/component" set to "timeseries"
    And the secret should have label "app.kubernetes.io/managed-by" set to "axonops-operator"
