Feature: Global imageRegistry override for on-premises deployments
  As a cluster administrator deploying AxonOps on-premises
  I need a single setting to redirect all component images to my private registry
  So that I can use a registry proxy (e.g., Harbor, Artifactory) without overriding each component individually

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And a namespace "axonops-test" exists

  # --- Happy path ---

  Scenario: Global imageRegistry overrides all default images
    Given an AxonOpsServer CR with imageRegistry "harbor.internal.com"
    And no per-component image overrides
    When the operator reconciles the AxonOpsServer CR
    Then the TimeSeries container should use image "harbor.internal.com/axonops/axondb-timeseries"
    And the Search container should use image "harbor.internal.com/axonops/axondb-search"
    And the Server container should use image "harbor.internal.com/axonops-public/axonops-docker/axon-server"
    And the Dashboard container should use image "harbor.internal.com/axonops-public/axonops-docker/axon-dash"
    And the init containers should use image "harbor.internal.com/busybox"

  Scenario: imageRegistry with port number
    Given an AxonOpsServer CR with imageRegistry "harbor.internal.com:5000"
    And no per-component image overrides
    When the operator reconciles the AxonOpsServer CR
    Then the TimeSeries container should use image "harbor.internal.com:5000/axonops/axondb-timeseries"
    And the Server container should use image "harbor.internal.com:5000/axonops-public/axonops-docker/axon-server"
    And the init containers should use image "harbor.internal.com:5000/busybox"

  Scenario: imageRegistry combined with custom tags
    Given an AxonOpsServer CR with imageRegistry "harbor.internal.com"
    And Server tag "2.0.30"
    And Dashboard tag "2.0.29"
    When the operator reconciles the AxonOpsServer CR
    Then the Server container should use image "harbor.internal.com/axonops-public/axonops-docker/axon-server:2.0.30"
    And the Dashboard container should use image "harbor.internal.com/axonops-public/axonops-docker/axon-dash:2.0.29"

  # --- Precedence ---

  Scenario: Per-component image override takes precedence over imageRegistry
    Given an AxonOpsServer CR with imageRegistry "harbor.internal.com"
    And Dashboard image "custom-registry.io/custom/axon-dash"
    And Dashboard tag "custom-tag"
    When the operator reconciles the AxonOpsServer CR
    Then the Dashboard container should use image "custom-registry.io/custom/axon-dash:custom-tag"
    And the TimeSeries container should use image "harbor.internal.com/axonops/axondb-timeseries"
    And the Server container should use image "harbor.internal.com/axonops-public/axonops-docker/axon-server"

  Scenario: Custom initImage takes precedence over imageRegistry
    Given an AxonOpsServer CR with imageRegistry "harbor.internal.com"
    And initImage "my-registry.io/custom-busybox:2.0"
    When the operator reconciles the AxonOpsServer CR
    Then the init containers should use image "my-registry.io/custom-busybox:2.0"
    And the TimeSeries container should use image "harbor.internal.com/axonops/axondb-timeseries"

  # --- Backward compatibility ---

  Scenario: Empty imageRegistry uses default images
    Given an AxonOpsServer CR with no imageRegistry set
    When the operator reconciles the AxonOpsServer CR
    Then the TimeSeries container should use image "ghcr.io/axonops/axondb-timeseries"
    And the Search container should use image "ghcr.io/axonops/axondb-search"
    And the Server container should use image "registry.axonops.com/axonops-public/axonops-docker/axon-server"
    And the Dashboard container should use image "registry.axonops.com/axonops-public/axonops-docker/axon-dash"

  # --- Edge cases ---

  Scenario: imageRegistry with trailing slash is normalized
    Given an AxonOpsServer CR with imageRegistry "harbor.internal.com/"
    When the operator reconciles the AxonOpsServer CR
    Then the TimeSeries container should use image "harbor.internal.com/axonops/axondb-timeseries"

  Scenario: imageRegistry change triggers rolling update
    Given an AxonOpsServer CR with imageRegistry "harbor-v1.internal.com"
    And all components are running
    When I update imageRegistry to "harbor-v2.internal.com"
    And the operator reconciles the AxonOpsServer CR
    Then all StatefulSets and Deployments should be updated with images from "harbor-v2.internal.com"
    And a rolling update should be triggered for affected pods
