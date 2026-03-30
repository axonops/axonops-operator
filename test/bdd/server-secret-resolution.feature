Feature: Server pod resolves correct auth secret names
  As an operator user
  I want the axon-server pod to reference the correct authentication secrets
  So that database credentials are properly injected regardless of reconcile timing

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And the AxonOps CRDs are registered

  Scenario: Server uses SecretRef for external TimeSeries on first reconcile
    Given an existing Secret named "my-ts-auth" with keys "AXONOPS_DB_USER" and "AXONOPS_DB_PASSWORD"
    And an AxonOpsPlatform CR with:
      - spec.timeSeries.external.hosts set to ["cassandra.example.com:9042"]
      - spec.timeSeries.authentication.secretRef set to "my-ts-auth"
      - spec.search as empty (internal defaults)
    When the operator reconciles the AxonOpsPlatform CR for the first time
    Then the axon-server pod env var "CQL_USERNAME" should reference secret "my-ts-auth"
    And the axon-server pod env var "CQL_PASSWORD" should reference secret "my-ts-auth"
    And the axon-server pod should NOT reference the default secret "<name>-timeseries-auth"

  Scenario: Server uses SecretRef for external Search on first reconcile
    Given an existing Secret named "my-search-auth" with keys "AXONOPS_SEARCH_USER" and "AXONOPS_SEARCH_PASSWORD"
    And an AxonOpsPlatform CR with:
      - spec.search.external.hosts set to ["https://elasticsearch.example.com:9200"]
      - spec.search.authentication.secretRef set to "my-search-auth"
      - spec.timeSeries as empty (internal defaults)
    When the operator reconciles the AxonOpsPlatform CR for the first time
    Then the axon-server pod env var "SEARCH_DB_USERNAME" should reference secret "my-search-auth"
    And the axon-server pod env var "SEARCH_DB_PASSWORD" should reference secret "my-search-auth"
    And the axon-server pod should NOT reference the default secret "<name>-search-auth"

  Scenario: Server uses SecretRef for both external components on first reconcile
    Given an existing Secret named "ts-creds" with keys "AXONOPS_DB_USER" and "AXONOPS_DB_PASSWORD"
    And an existing Secret named "search-creds" with keys "AXONOPS_SEARCH_USER" and "AXONOPS_SEARCH_PASSWORD"
    And an AxonOpsPlatform CR with:
      - spec.timeSeries.external.hosts set to ["cassandra:9042"]
      - spec.timeSeries.authentication.secretRef set to "ts-creds"
      - spec.search.external.hosts set to ["https://elastic:9200"]
      - spec.search.authentication.secretRef set to "search-creds"
    When the operator reconciles the AxonOpsPlatform CR for the first time
    Then the axon-server pod env var "CQL_USERNAME" should reference secret "ts-creds"
    And the axon-server pod env var "CQL_PASSWORD" should reference secret "ts-creds"
    And the axon-server pod env var "SEARCH_DB_USERNAME" should reference secret "search-creds"
    And the axon-server pod env var "SEARCH_DB_PASSWORD" should reference secret "search-creds"

  Scenario: Server falls back to default secret name when no SecretRef is set
    Given an AxonOpsPlatform CR named "myapp" with:
      - spec.timeSeries.authentication.username set to "cassandra"
      - spec.timeSeries.authentication.password set to "password123"
      - spec.search.authentication.username set to "elastic"
      - spec.search.authentication.password set to "password456"
    When the operator reconciles the AxonOpsPlatform CR
    Then the axon-server pod env var "CQL_USERNAME" should reference secret "myapp-timeseries-auth"
    And the axon-server pod env var "CQL_PASSWORD" should reference secret "myapp-timeseries-auth"
    And the axon-server pod env var "SEARCH_DB_USERNAME" should reference secret "myapp-search-auth"
    And the axon-server pod env var "SEARCH_DB_PASSWORD" should reference secret "myapp-search-auth"

  Scenario: SecretRef is honoured consistently across multiple reconcile loops
    Given an existing Secret named "external-ts-auth" with keys "AXONOPS_DB_USER" and "AXONOPS_DB_PASSWORD"
    And an AxonOpsPlatform CR with:
      - spec.timeSeries.external.hosts set to ["cassandra:9042"]
      - spec.timeSeries.authentication.secretRef set to "external-ts-auth"
    When the operator reconciles the AxonOpsPlatform CR for the first time
    And the operator reconciles the AxonOpsPlatform CR a second time
    Then the axon-server pod env var "CQL_USERNAME" should reference secret "external-ts-auth" after both reconciles
    And the axon-server pod env var "CQL_PASSWORD" should reference secret "external-ts-auth" after both reconciles

  Scenario: Changing from inline credentials to SecretRef updates the server pod
    Given an AxonOpsPlatform CR named "myapp" with inline TimeSeries credentials
    And the axon-server pod references secret "myapp-timeseries-auth"
    When the user updates the CR to set spec.timeSeries.authentication.secretRef to "new-ts-secret"
    And the operator reconciles the AxonOpsPlatform CR
    Then the axon-server pod env var "CQL_USERNAME" should reference secret "new-ts-secret"
    And the axon-server pod env var "CQL_PASSWORD" should reference secret "new-ts-secret"
