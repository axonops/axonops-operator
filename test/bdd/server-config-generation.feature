Feature: Server and Dashboard configuration generation
  As a Kubernetes operator
  I need to generate correct configuration files for axon-server and axon-dash
  So that the components start with proper settings for the deployment mode

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And a namespace "axonops-test" exists

  Scenario: Server config contains correct ports
    Given an AxonOpsPlatform CR with all components enabled
    When the operator reconciles the AxonOpsPlatform CR
    Then the Server config Secret should contain axon-server.yml
    And the config should set "agents_port" to 1888
    And the config should set "api_port" to 8080
    And the config should set "host" to "0.0.0.0"

  Scenario: Server config contains internal database URLs
    Given an AxonOpsPlatform CR with internal TimeSeries and Search
    When the operator reconciles the AxonOpsPlatform CR
    Then the Server config should reference the internal TimeSeries headless Service FQDN for CQL hosts
    And the Server config should reference the internal Search headless Service FQDN for search_db hosts

  Scenario: Server config contains external database URLs
    Given an AxonOpsPlatform CR with external TimeSeries hosts "cassandra-1.example.com,cassandra-2.example.com"
    And external Search hosts "opensearch-1.example.com"
    When the operator reconciles the AxonOpsPlatform CR
    Then the Server config should contain the external TimeSeries hosts for CQL
    And the Server config should contain the external Search host for search_db

  Scenario: Server config includes TLS settings for internal databases
    Given an AxonOpsPlatform CR with internal TimeSeries and Search with TLS enabled
    When the operator reconciles the AxonOpsPlatform CR
    Then the Server config should set "cql_ssl" to true
    And the Server config should reference the CA certificate path

  Scenario: Server config includes organization name
    Given an AxonOpsPlatform CR with orgName "my-organization"
    When the operator reconciles the AxonOpsPlatform CR
    Then the Server config should set "org_name" to "my-organization"

  Scenario: Server config log output set for Kubernetes
    Given an AxonOpsPlatform CR with default configuration
    When the operator reconciles the AxonOpsPlatform CR
    Then the Server config should set "log_file" to "/dev/stdout"

  Scenario: Dashboard config contains server endpoint
    Given an AxonOpsPlatform CR with all components enabled
    When the operator reconciles the AxonOpsPlatform CR
    Then the Dashboard ConfigMap should contain axon-dash.yml
    And the config should reference the Server API Service for private_endpoints
    And the config should set dashboard host to "0.0.0.0"
    And the config should set dashboard port to 3000

  Scenario: Dashboard config does not enable SSL
    Given an AxonOpsPlatform CR with Dashboard enabled
    When the operator reconciles the AxonOpsPlatform CR
    Then the Dashboard ConfigMap should set "ssl.enabled" to false
    Because TLS is handled by Ingress or Gateway, not the Dashboard itself
