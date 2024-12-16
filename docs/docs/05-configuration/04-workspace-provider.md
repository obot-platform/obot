# Workspace Provider

In Acorn, a workspace is where files are stored and manipulated by a user. By default, any workspace is a directory on local disk. However, is some server-based applications, this is not acceptable. The concept of a workspace provider is used to abstract away the concept of a workspace and use other options.

This section describes the configuration of the workspace provider.

#### `ACORN_WORKSPACE_PROVIDER_TYPE`

The type of provider to use. The current options are `directory` or `s3`. Note that the `s3` provider is compatible with s3-compatible services like CloudFlare R2.

### The directory provider configuration

#### `WORKSPACE_PROVIDER_DATA_HOME`

Specify the directory where workspaces are nested. The default is `$XDG_CONFIG_HOME/acorn-io/workspace-provider`.

### The s3 provider configuration

#### `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION`, `WORKSPACE_PROVIDER_S3_BUCKET`

Specifying these variables allows you to configure access to an s3 or s3-compatible bucket for the workspace provider to use. If the `ACORN_WORKSPACE_PROVIDER_TYPE` is `s3`, then all of these variables are required for proper configuration.

#### `WORKSPACE_PROVIDER_S3_BASE_ENDPOINT`

This is necessary for using an s3-compatible service like CloudFlare R2.
