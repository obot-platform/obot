# OAuth Apps

OAuth 2.0 (hereon referred to as simply OAuth) is an open standard for authorization, designed to allow applications to access a user’s resources on another service without sharing their password. It enables a secure and standardized way for apps to connect to external services. 

Otto8 makes use of OAuth to allow agents to interact with systems and services on behalf of the user. For example, through OAuth, an agent can check a user's Slack messages or even send a message as the user.

If an Otto8 tool needs to talk to an external service and wants to use Oauth to do so, it will need a corresponding Oauth App. The built-in OAuth Apps corresponding to our built-in tools.

Because configuring an OAuth integration can be complicated, Otto8's built-in Oauth Apps are pre-configured to use a public gateway and ran by Acorn Labs. We've configured a corresponding OAuth application in each service provider (ie, GitHub, Google, and Microsoft) so that you can start using the tools without any additional configuration.  Any of these integrations can be overridden to use your own custom integration. Each service provider is different, but you just need to follow the in-app instructions to configure them properly.

### Custom OAuth Apps
You can configure a custom OAuth app to integrate with any service provider that supports OAuth. You can then use this when authoring your own custom tools that call that service. For more details, see our [Tool Authoring Guide](/guides/integrating-oauth).

When configuring a custom OAuth App, you'll first be asked to provide the following values:
- **Name** - This is just the friendly name you'll use to identify your app
- **Integration** - This value will be used when write your custom tool. It will be used to link that tool to your OAuth app
- **Authorization URL** - This will be supplied by the service provider. Each service provider is different, so you'll have to follow their instructions for obtaining an authorization URL.
- **Token URL** - Like the Authorization URL, this will be supplied by the service provider.

Once you supply these values, you'll be given a **Redirect URL**, which the service provider will want in its configuration.

Finally, you'll be asked for the **Client ID** and **Client Secret**. Both of these will be supplied by the service provider, typically after you've supplied it with the Redirect URL.