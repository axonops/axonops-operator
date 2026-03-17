Feature: Pod customization with annotations, labels, environment variables, and volumes
  As a cluster administrator
  I need to add custom annotations, labels, environment variables, and volumes to component pods
  So that I can integrate with monitoring, service mesh, and other infrastructure tools

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And a namespace "axonops-test" exists

  Scenario: Custom annotations on component pods
    Given an AxonOpsServer CR with custom annotations on the Server component:
      | key                              | value   |
      | prometheus.io/scrape             | "true"  |
      | prometheus.io/port               | "8080"  |
    When the operator reconciles the AxonOpsServer CR
    Then the Server StatefulSet pod template should have annotation "prometheus.io/scrape" with value "true"
    And the Server StatefulSet pod template should have annotation "prometheus.io/port" with value "8080"

  Scenario: Custom labels on component pods
    Given an AxonOpsServer CR with custom labels on the Dashboard component:
      | key         | value      |
      | team        | platform   |
      | environment | production |
    When the operator reconciles the AxonOpsServer CR
    Then the Dashboard Deployment pod template should have label "team" with value "platform"
    And the Dashboard Deployment pod template should have label "environment" with value "production"

  Scenario: Custom environment variables on Dashboard
    Given an AxonOpsServer CR with Dashboard environment variables:
      | name          | value              |
      | CUSTOM_HEADER | "My Custom Header" |
      | LOG_LEVEL     | "debug"            |
    When the operator reconciles the AxonOpsServer CR
    Then the Dashboard container should have environment variable "CUSTOM_HEADER" set to "My Custom Header"
    And the Dashboard container should have environment variable "LOG_LEVEL" set to "debug"

  Scenario: Extra volumes and volume mounts on Server
    Given an AxonOpsServer CR with extra volumes on the Server component:
      | volume_name | type      | source           |
      | custom-certs| configMap | my-custom-certs  |
    And extra volume mounts:
      | volume_name  | mountPath           | readOnly |
      | custom-certs | /etc/custom-certs   | true     |
    When the operator reconciles the AxonOpsServer CR
    Then the Server StatefulSet should have a volume "custom-certs" from ConfigMap "my-custom-certs"
    And the Server container should have a volume mount at "/etc/custom-certs" that is read-only

  Scenario: Extra volumes and volume mounts on Dashboard
    Given an AxonOpsServer CR with extra volumes on the Dashboard component:
      | volume_name | type   | source          |
      | branding    | secret | branding-assets |
    And extra volume mounts:
      | volume_name | mountPath         | readOnly |
      | branding    | /app/branding     | true     |
    When the operator reconciles the AxonOpsServer CR
    Then the Dashboard Deployment should have a volume "branding" from Secret "branding-assets"
    And the Dashboard container should have a volume mount at "/app/branding" that is read-only
