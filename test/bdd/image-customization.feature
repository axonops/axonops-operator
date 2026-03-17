Feature: Image customization for AxonOpsServer components
  As a cluster administrator
  I need to configure custom container images, tags, and pull policies per component
  So that I can use private registries or specific versions

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And a namespace "axonops-test" exists

  Scenario: Default images are used when no overrides specified
    Given an AxonOpsServer CR with no image overrides
    When the operator reconciles the AxonOpsServer CR
    Then the TimeSeries container should use image "ghcr.io/axonops/axondb-timeseries:5.0.6-1.1.0"
    And the Search container should use image "ghcr.io/axonops/axondb-search:3.3.2-1.5.0"
    And the Server container should use image "registry.axonops.com/axonops-public/axonops-docker/axon-server:2.0.27"
    And the Dashboard container should use image "registry.axonops.com/axonops-public/axonops-docker/axon-dash:2.0.28"

  Scenario: Custom image repository for TimeSeries
    Given an AxonOpsServer CR with TimeSeries image "my-registry.com/custom-timeseries"
    And TimeSeries tag "1.0.0"
    When the operator reconciles the AxonOpsServer CR
    Then the TimeSeries container should use image "my-registry.com/custom-timeseries:1.0.0"

  Scenario: Custom image repository for Search
    Given an AxonOpsServer CR with Search image "my-registry.com/custom-search"
    And Search tag "2.0.0"
    When the operator reconciles the AxonOpsServer CR
    Then the Search container should use image "my-registry.com/custom-search:2.0.0"

  Scenario: Custom image repository for Server
    Given an AxonOpsServer CR with Server image "my-registry.com/custom-server"
    And Server tag "3.0.0"
    When the operator reconciles the AxonOpsServer CR
    Then the Server container should use image "my-registry.com/custom-server:3.0.0"

  Scenario: Custom image repository for Dashboard
    Given an AxonOpsServer CR with Dashboard image "my-registry.com/custom-dash"
    And Dashboard tag "4.0.0"
    When the operator reconciles the AxonOpsServer CR
    Then the Dashboard container should use image "my-registry.com/custom-dash:4.0.0"

  Scenario: Custom pull policy per component
    Given an AxonOpsServer CR with TimeSeries pullPolicy "Always"
    And Search pullPolicy "Never"
    And Server pullPolicy "IfNotPresent"
    When the operator reconciles the AxonOpsServer CR
    Then the TimeSeries container should have imagePullPolicy "Always"
    And the Search container should have imagePullPolicy "Never"
    And the Server container should have imagePullPolicy "IfNotPresent"

  Scenario: Update image tag triggers rolling update
    Given an AxonOpsServer CR with Server tag "2.0.27"
    And the Server StatefulSet is running
    When I update the Server tag to "2.0.28"
    And the operator reconciles the AxonOpsServer CR
    Then the Server StatefulSet should be updated with the new image tag
    And a rolling update should be triggered for the Server pods
