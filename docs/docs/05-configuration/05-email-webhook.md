# Email Webhook Configuration

Obot can be configured to receive email and trigger workflows. Currently, it only supports receiving email via SendGrid's Inbound Parse Webhook. This allows Obot to trigger workflows based on the content of incoming emails. We are working on adding support for other email services.

### Supported Email Services

- SendGrid

### SendGrid

#### Configure Obot

You need to configure Obot with an email server name where it is going to receive emails. The email server name should match the domain you have configured in SendGrid. For example, if you want to receive emails from `john@yourcompany.com`, you need to configure Obot with `yourcompany.com`.

You can configure the email server name by setting the `OBOT_EMAIL_SERVER_NAME` environment variable.

#### Configuring SendGrid Inbound Parse API

To configure SendGrid to forward emails to Obot, follow these steps:

1. Log in to your SendGrid account.
2. Navigate to the Inbound Parse settings.
3. Add a new Inbound Parse Webhook.
4. Set the domain to the email server name you configured in Obot.
5. Set the webhook URL to `https://{obot_server_url}/api/sendgrid`. Replace `{obot_server_url}` with the actual URL of your Obot server.

To see more details, follow the detailed instructions provided by SendGrid [here](https://www.twilio.com/docs/sendgrid/for-developers/parsing-email/setting-up-the-inbound-parse-webhook).

:::note
By default, Sendgrid inbound webhook does not provide a way to verify the signature and payload coming from SendGrid. In order to verify requests that are coming from sendgird, Obot provides a way to configure both `OBOT_SENDGRID_WEBHOOK_USERNAME` and `OBOT_SENDGRID_WEBHOOK_PASSWORD` to enable basic authentication for the webhook endpoint. So you can configure url with basic auth `https://{username}:{password}@{obot_server_url}/api/sendgrid` to ensure that only requests from SendGrid are received in Obot. For more details, refer to this example: [Handling SendGrid Inbound Parse](https://www.twilio.com/en-us/blog/microservice-template-handle-sendgrid-inbound-parse). It is recommended to configure these credentials to secure the endpoint and protect it from unverified payloads.

To do this,

1. Set `OBOT_SENDGRID_WEBHOOK_USERNAME` and `OBOT_SENDGRID_WEBHOOK_PASSWORD` environment variables when you start Obot.
2. Configure SendGrid webhook URL with basic auth `https://{username}:{password}@{obot_server_url}/api/sendgrid`.
   :::

#### Triggering Workflows

Once the SendGrid Inbound Parse Webhook is configured, emails that are forwarded to your email address will also be forwarded to the Obot server, triggering workflows based on the email reply address. To learn more about how emails can trigger workflows, follow the documentation [here](/concepts/workflows#email).
