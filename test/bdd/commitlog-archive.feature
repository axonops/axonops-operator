Feature: AxonOpsCommitlogArchive lifecycle for Cassandra commitlog archive settings
  As an operator user
  I want to declare commitlog archive settings using AxonOpsCommitlogArchive CRs
  So that commitlog archiving is managed declaratively via GitOps

  Background:
    Given a Kubernetes cluster with the AxonOps operator installed
    And the AxonOps CRDs are registered
    And a valid AxonOpsConnection CR "my-connection" exists in namespace "default"

  # --- Local remote type ---

  Scenario: Create commitlog archive with local storage
    Given an AxonOpsCommitlogArchive CR with:
      | field           | value            |
      | connectionRef   | my-connection    |
      | clusterName     | my-cluster       |
      | clusterType     | cassandra        |
      | remoteType      | local            |
      | remotePath      | /mnt/commitlogs  |
      | datacenters     | ["dc1"]          |
    When the operator reconciles the AxonOpsCommitlogArchive CR
    Then the AxonOps API should receive a POST to /api/v1/cassandraCommitLogsSettings
    And the payload remoteType should be "local"
    And the payload remoteConfig should contain "type = local"
    And the status condition "Ready" should be "True"

  # --- S3 remote type ---

  Scenario: Create commitlog archive with S3 and explicit credentials
    Given a Secret "s3-creds" with keys "access_key_id" and "secret_access_key"
    And an AxonOpsCommitlogArchive CR with:
      | field           | value                       |
      | connectionRef   | my-connection               |
      | clusterName     | my-cluster                  |
      | clusterType     | cassandra                   |
      | remoteType      | s3                          |
      | remotePath      | s3://my-bucket/commitlogs   |
      | remoteRetention | 90d                         |
      | datacenters     | ["dc1", "dc2"]              |
    And the S3 config has region="us-east-1" and credentialsRef pointing to "s3-creds"
    When the operator reconciles the AxonOpsCommitlogArchive CR
    Then the AxonOps API should receive a POST with:
      | field              | value                     |
      | remoteType         | s3                        |
      | remotePath         | s3://my-bucket/commitlogs |
      | AWSRegion          | us-east-1                 |
    And the payload remoteConfig should contain "env_auth = false"
    And the payload remoteConfig should contain "access_key_id ="
    And the status condition "Ready" should be "True"

  Scenario: Create commitlog archive with S3 IAM role auth (no credentials)
    Given an AxonOpsCommitlogArchive CR with remoteType="s3" and no credentialsRef
    When the operator reconciles the AxonOpsCommitlogArchive CR
    Then the payload remoteConfig should contain "env_auth = true"
    And the payload should NOT contain AWSAccessKeyId
    And the status condition "Ready" should be "True"

  Scenario: Create commitlog archive with S3 custom storage class and encryption
    Given an AxonOpsCommitlogArchive CR with remoteType="s3"
    And S3 config with storageClass="GLACIER" and encryption="none" and acl="bucket-owner-full-control"
    When the operator reconciles the AxonOpsCommitlogArchive CR
    Then the payload AWSStorageClass should be "GLACIER"
    And the payload AWSServerSideEncryption should be "none"
    And the payload AWSACL should be "bucket-owner-full-control"

  # --- SFTP remote type ---

  Scenario: Create commitlog archive with SFTP and password auth
    Given a Secret "sftp-creds" with key "password" containing the SSH password
    And an AxonOpsCommitlogArchive CR with:
      | field           | value                   |
      | remoteType      | sftp                    |
      | remotePath      | /backup/commitlogs      |
      | datacenters     | ["dc1"]                 |
    And the SFTP config has host="sftp.example.com", user="backup", credentialsRef with passwordKey="password"
    When the operator reconciles the AxonOpsCommitlogArchive CR
    Then the payload SFTPHost should be "sftp.example.com"
    And the payload SFTPUser should be "backup"
    And the payload remoteConfig should contain "type = sftp"
    And the status condition "Ready" should be "True"

  Scenario: Create commitlog archive with SFTP and key file auth
    Given a Secret "sftp-key" with key "private_key" containing the SSH private key
    And an AxonOpsCommitlogArchive CR with SFTP config using keyFileKey="private_key"
    When the operator reconciles the AxonOpsCommitlogArchive CR
    Then the payload remoteConfig should contain "key_file ="
    And the status condition "Ready" should be "True"

  # --- Update and no-op ---

  Scenario: Update commitlog archive settings
    Given an existing AxonOpsCommitlogArchive CR with Ready=True and remoteRetention="60d"
    When the user updates remoteRetention to "90d"
    And the operator reconciles the AxonOpsCommitlogArchive CR
    Then the AxonOps API should receive a DELETE to remove old settings
    And the AxonOps API should receive a POST with remoteRetentionDuration="90d"
    And the status condition "Ready" should be "True"

  Scenario: No API call when settings have not changed
    Given an existing AxonOpsCommitlogArchive CR with Ready=True
    And the status observedGeneration matches the CR generation
    When the operator reconciles the AxonOpsCommitlogArchive CR
    Then no API call should be made
    And the status should remain unchanged

  # --- Deletion ---

  Scenario: Delete commitlog archive CR removes settings from API
    Given an existing AxonOpsCommitlogArchive CR with Ready=True
    When the user deletes the AxonOpsCommitlogArchive CR
    And the operator reconciles the deletion
    Then the AxonOps API should receive a DELETE to /api/v1/cassandraCommitLogsSettings
    And the finalizer should be removed
    And the CR should be deleted from Kubernetes

  Scenario: Delete when API unreachable retries without removing finalizer
    Given an existing AxonOpsCommitlogArchive CR with Ready=True
    And the AxonOps API is unreachable
    When the user deletes the AxonOpsCommitlogArchive CR
    Then the finalizer should NOT be removed
    And the CR should be requeued for retry after 30 seconds

  # --- Validation errors ---

  Scenario: Failed condition when remoteType=s3 but no S3 config provided
    Given an AxonOpsCommitlogArchive CR with remoteType="s3" and no s3 config block
    When the operator reconciles the AxonOpsCommitlogArchive CR
    Then the status condition "Failed" should be "True" with reason "InvalidConfig"

  Scenario: Failed condition when remoteType=sftp but no SFTP config provided
    Given an AxonOpsCommitlogArchive CR with remoteType="sftp" and no sftp config block
    When the operator reconciles the AxonOpsCommitlogArchive CR
    Then the status condition "Failed" should be "True" with reason "InvalidConfig"

  Scenario: Failed condition when S3 credentials Secret not found
    Given an AxonOpsCommitlogArchive CR with S3 credentialsRef pointing to "nonexistent"
    When the operator reconciles the AxonOpsCommitlogArchive CR
    Then the status condition "Failed" should be "True" with reason "SecretNotFound"
    And the CR should be requeued for retry after 30 seconds

  Scenario: Failed condition when AxonOpsConnection not found
    Given an AxonOpsCommitlogArchive CR with connectionRef="nonexistent"
    When the operator reconciles the AxonOpsCommitlogArchive CR
    Then the status condition "Failed" should be "True" with reason "ConnectionError"
    And the CR should be requeued for retry after 30 seconds

  # --- API errors ---

  Scenario: AxonOps API returns server error
    Given an AxonOpsCommitlogArchive CR with a valid connection
    And the AxonOps API returns HTTP 500
    When the operator reconciles the AxonOpsCommitlogArchive CR
    Then the status condition "Failed" should be "True" with reason "APIError"
    And the CR should be requeued for retry after 30 seconds

  Scenario: AxonOps API returns client error
    Given an AxonOpsCommitlogArchive CR with a valid connection
    And the AxonOps API returns HTTP 400
    When the operator reconciles the AxonOpsCommitlogArchive CR
    Then the status condition "Failed" should be "True" with reason "APIError"
    And the CR should NOT be requeued
