# Email Webhook Configuration

Obot can be configured to receive email and trigger tasks. Currently, it only supports receiving email via SendGrid's Inbound Parse Webhook. This allows Obot to trigger tasks based on the content of incoming emails.

## Supported Email Services

- SendGrid

:::Note
Additional integrations are planned.
:::

## SendGrid Integration

### 1. Configure Obot

You need to configure Obot with an email server name where it will receive emails. The email server name should match the domain configured in SendGrid.

- **Example**: If you want to receive emails from `john@yourcompany.com`, configure Obot with `yourcompany.com`
- **Configuration**: Set the `OBOT_SERVER_EMAIL_SERVER_NAME` environment variable with your domain

### 2. Configure SendGrid Inbound Parse API

Follow these steps to set up SendGrid to forward emails to Obot:

1. Log in to your SendGrid account
2. Navigate to the Inbound Parse settings
3. Click on `Add Host & Url`
4. **Receiving Domain**: Set the domain to the email server name you configured in Obot (e.g., `yourcompany.com`)
5. **Destination URL**: Set to `https://{obot_server_url}/api/sendgrid` (replace `{obot_server_url}` with your actual Obot server URL)

For detailed instructions, see the [SendGrid documentation](https://www.twilio.com/docs/sendgrid/for-developers/parsing-email/setting-up-the-inbound-parse-webhook).

### 3. Secure Your Webhook (Recommended)

:::note
By default, SendGrid inbound webhook does not provide a way to verify the signature and payload. To ensure only authentic SendGrid requests are processed, Obot supports basic authentication for the webhook endpoint.

**To enable basic authentication:**

1. Set these environment variables when starting Obot:
   - `OBOT_SERVER_SENDGRID_WEBHOOK_USERNAME`
   - `OBOT_SERVER_SENDGRID_WEBHOOK_PASSWORD`

2. Configure SendGrid webhook URL with basic auth:

   ```
   https://{username}:{password}@{obot_server_url}/api/sendgrid
   ```

For more details, see this example: [Handling SendGrid Inbound Parse](https://www.twilio.com/en-us/blog/microservice-template-handle-sendgrid-inbound-parse).
:::
