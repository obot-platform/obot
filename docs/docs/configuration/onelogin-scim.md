---
displayed_sidebar: sidebar
title: "Provision users and groups from OneLogin with SCIM"
---

# Provision users and groups from OneLogin with SCIM

With SCIM, OneLogin sends Obot your users, their roles, and their account
status. You decide in OneLogin exactly which users and roles Obot can see, and
Obot does not need an API credential for your OneLogin account.

Users still sign in through the OneLogin OIDC app described in
[OneLogin](./auth-providers.md#onelogin). SCIM uses a second OneLogin app that
only sends user and role changes to Obot. It is never used to sign in.

SCIM requires OneLogin Identity Lifecycle Management. Only an Obot Owner can set
it up.

:::note OneLogin must be able to reach Obot
OneLogin sends SCIM requests to Obot from OneLogin's own servers, so
`https://<your-obot-url>/scim/v2` and all routes under it must be reachable from
OneLogin. If Obot is on a private network or behind a firewall, allow OneLogin
to reach those routes, or provisioning will fail.
:::

## How SCIM changes user management

- **OneLogin controls who has access.** Giving a user access to the SCIM app
  gives them an Obot account. Removing their access, suspending them, or
  deleting them in OneLogin deactivates them in Obot: they are signed out, and
  their API keys and other credentials stop working. Their account, data, and
  groups are kept, and restoring their access reactivates the same account.
- **OneLogin roles become groups in Obot.** Only roles that you push from OneLogin
  appear in Obot. Role changes reach Obot within seconds, instead of when users
  next sign in.
- **Enforcing SCIM limits sign-in to provisioned users.** Until you enforce
  SCIM, anyone who can sign in through the OneLogin OIDC app can still use Obot.
- **SCIM stays on.** You can't switch OneLogin back to an API credential without
  removing the OneLogin auth provider. See [Stop using SCIM](#stop-using-scim).

:::warning Unlicensing a user does not remove their access
OneLogin doesn't notify Obot when a user is unlicensed. The user can't sign in
to OneLogin, but stays active in Obot, and their existing sessions and API keys
keep working. To remove someone's access to Obot, remove their access to the
SCIM app, or suspend them.
:::

## Choose your setup path

- If you haven't configured OneLogin in Obot yet, follow [Set up OneLogin with
  SCIM](#set-up-onelogin-with-scim).
- If OneLogin is already your auth provider, configured with an API credential,
  follow [Move an existing OneLogin setup to
  SCIM](#move-an-existing-onelogin-setup-to-scim).

## Set up OneLogin with SCIM

### Step 1: Configure OneLogin without an API credential

1. Create the OneLogin OIDC app as described in
   [OneLogin](./auth-providers.md#onelogin). Skip the API credential.
2. In Obot, go to **Identity & Access > Auth Providers** and configure
   **OneLogin**.
3. Fill in **Client ID**, **Client Secret**, **Issuer URL**, and **Allowed
   E-Mail Domains**. Leave **API Client ID** and **API Client Secret** empty.
   Leaving them empty tells Obot to use SCIM.
4. Sign in to Obot through OneLogin as the account that will be the Owner.

### Step 2: Generate the SCIM token

1. Go to **Identity & Access > Auth Providers > SCIM**.
2. Select **Generate token**.
3. Copy the **Base URL** and the token. The token is shown only once.

### Step 3: Create the SCIM app in OneLogin

Follow [Create the SCIM app in OneLogin](#create-the-scim-app-in-onelogin) using
the base URL and token from step 2. Make sure your own account has access to the
app.

### Step 4: Push roles

Set up the rule that pushes roles as groups. See [Push roles](#push-roles).

Pushed roles appear in Obot as groups, which you can use in roles and access
policies like any other group.

### Step 5: Enforce SCIM

1. Return to **Identity & Access > Auth Providers > SCIM**.
2. Review the users OneLogin has provisioned. Users listed under **Not
   provisioned** will be disabled when you enforce. Give them access to the SCIM
   app in OneLogin if they should keep access.
3. Select **Enforce SCIM**.

From now on, only users that OneLogin has provisioned can sign in. Enforcing
can't be undone.

## Move an existing OneLogin setup to SCIM

Moving to SCIM keeps your existing users, groups, roles, and policies. Users
keep their accounts and data, and groups keep their roles and policy assignments
once OneLogin pushes them.

### Step 1: Review the move in Obot

Go to **Identity & Access > Auth Providers > SCIM**. The **Move OneLogin to
SCIM** panel lists anything you need to fix first, and what will change.

If two OneLogin roles that roles, policies, or
[vMCP profiles](../mcp-gateway/access.md#virtual-mcps-tools-and-profiles) use
have the same name, remove the references to all but one, or rename one of them
in OneLogin. Obot compares names ignoring case and surrounding whitespace, so
names must be unique under that comparison. After renaming, allow up to 15
minutes for Obot's OneLogin provider to refresh its role names, then have a
member of the renamed role sign in to Obot. Verify that the duplicate-name
blocker disappears before enabling SCIM.

### Step 2: Enable SCIM

1. Select **Enable SCIM** and confirm.
2. Copy the **Base URL** and the token. The token is shown only once.

When you enable SCIM:

- Everyone can still sign in.
- Obot stops reading roles from OneLogin. Until a role is pushed, its group
  keeps its current members, including anyone you remove from the role in
  OneLogin. Push your roles soon after enabling.
- Groups from OneLogin that no role, policy, or vMCP profile uses are removed from
  Obot. They grant nothing, so no one loses access, and they come back when
  OneLogin pushes them.

Enabling SCIM can't be undone.

### Step 3: Create the SCIM app in OneLogin

Follow [Create the SCIM app in OneLogin](#create-the-scim-app-in-onelogin) using
the base URL and token from step 2. Give it access to the same roles as your
Obot OIDC app.

### Step 4: Push the roles Obot lists

Set up the rule in [Push roles](#push-roles) so that it pushes every role listed
under **Referenced groups not pushed yet** on the Obot SCIM page. Each one links
to its page in OneLogin.

OneLogin never pushes a role that has no members. If a listed role is empty,
give it a member in OneLogin, or remove it from the roles, policies, and vMCP
profiles that use it.

### Step 5: Enforce SCIM

1. Return to **Identity & Access > Auth Providers > SCIM** and review the
   **Enforce** step. It lists anything that still blocks enforcing, such as
   roles that haven't been pushed.
2. Review the users under **Not provisioned**. They will be disabled when you
   enforce. Give them access to the SCIM app in OneLogin if they should keep
   access. Disabled users keep their accounts and data, and provisioning them
   later re-enables them.
3. Select **Enforce SCIM**.

To enforce, you must have signed in through OneLogin and been provisioned by the
SCIM app. This makes sure you can still sign in afterward.

### Step 6: Remove the API credential (optional)

Obot no longer uses the API credential. Modify the OneLogin auth provider to
clear **API Client ID** and **API Client Secret**, then delete the credential in
OneLogin.

## Create the SCIM app in OneLogin

You'll need your Obot URL, and the SCIM base URL and token from Obot.

:::warning Finish this setup before giving anyone access
OneLogin doesn't apply later changes to these settings to users it has already
provisioned. If any of them is wrong, OneLogin's provisioning events show an
error from Obot that says which setting to fix.
:::

1. In the OneLogin admin console, go to **Applications > Applications > Add
   App**, and add **SCIM Provisioner with SAML (SCIM v2 Core)**.
2. On the **Info** tab, turn off **Visible in portal**. The app never signs
   anyone in, and opening it from the OneLogin portal shows an error from Obot.
3. On the **Configuration** tab:
   - Set every SAML field to `https://<your-obot-url>/scim-provisioning`.
     OneLogin requires them, but Obot doesn't use them. If someone opens this
     page, it explains that the app is for provisioning only and links to the
     regular Obot sign-in.
   - Set **SCIM Base URL** to the base URL from Obot, and **SCIM Bearer Token**
     to the token.
   - In the **SCIM JSON Template**, add `"externalId": "{$user.id}",` after the
     `userName` line. Leave the `userName` line as it is. Obot uses `externalId`
     to match each user to their OneLogin sign-in.
   - Save, then select **Enable** under **API Connection**.
4. On the **Provisioning** tab:
   - Turn on **Enable provisioning**.
   - Set both **When users are deleted in OneLogin, or the user's app access is
     removed** and **When user accounts are suspended in OneLogin** to
     **Suspend**. Obot never deletes users over SCIM. To delete a suspended
     user, delete them on Obot's **Users** page. With **Do Nothing**, users you
     suspend in OneLogin stay active in Obot.
   - Turn off **Require admin approval** for each action, unless you want to
     approve every change by hand.
5. On the **Parameters** tab:
   - Set **SCIM Username** to **OneLogin ID**.
   - Open **Groups** and turn on **Include in User Provisioning**.
6. Set up the rule in [Push roles](#push-roles).
7. On the **Access** tab, select the roles whose users should be able to use
   Obot. Give the OIDC app the same roles, or provisioned users can't sign in.

On the Obot SCIM page, the **SCIM app** step is marked done once Obot receives
its first request from OneLogin.

### Push roles

OneLogin pushes a user's roles as groups through a rule.

1. On the SCIM app's **Rules** tab, add a rule with the action **Set Groups in
   \<app name>**, mapped from OneLogin, for each **role** with a value that
   matches a pattern. Use a pattern with no capture group, such as `.*` for
   every role, or `obot-(?:admins|users)` for some. A capture group, such as
   `obot-(admins|users)`, changes the names of the pushed groups, so they no
   longer match existing groups in Obot.
2. Save the rule, then select **More Actions > Reapply entitlement mappings**.
   Run it again after every change to the rule.

## Manage users and groups with SCIM

| Desired Action | Do this in OneLogin |
| --- | --- |
| Give someone access | Give them a role that has access to the SCIM app. |
| Remove someone's access | Remove their access to the SCIM app, suspend them, or delete them. |
| Add a role to Obot | Make sure the rule matches it, and give it at least one member. |
| Change a user's roles | Make the change in OneLogin. Obot is updated within seconds. |

A provisioned user's status in Obot is managed by OneLogin and can't be changed
on the **Users** page. To delete a user's Obot account and data permanently,
first remove their access in OneLogin, then delete them from the **Users** page
in Obot.

### What OneLogin doesn't send

- **Role renames.** Renaming a role sends nothing until you select **More
  Actions > Reapply entitlement mappings** on the SCIM app. The reapply creates
  a new group in Obot and moves the members, leaving the old group empty. Until
  then, the old group keeps its members and grants. Roles, policies, and vMCP
  profiles that use the old group don't move to the new one, so update them to
  use the new group.
- **Role deletes.** Deleting a role empties its group in Obot after the next
  **Reapply entitlement mappings**. The empty group stays in Obot and grants
  nothing.
- **Empty roles.** A role with no members is never pushed.
- **Roles removed while a user's access is removed.** If a user loses a role
  while their access to the SCIM app is removed, OneLogin never sends the
  change, even after their access is restored, so they stay in that group in
  Obot. After restoring a user's access, give them each role they lost back,
  then remove it again. Roles they gained in that time are sent when their
  access is restored. Suspended users are not affected.

### Retry failed changes

OneLogin retries a failed change only if Obot asked it to slow down. Every other
failure waits until you retry it, and blocks that user's later changes until
then. The **Activity** section of the Obot SCIM page lists recent failures.

To retry, select **Retry** on the user in the SCIM app's **Users** tab, or on the
app in the user's **Applications** tab.

Common errors:

- **"Obot does not delete users."** **When users are deleted in OneLogin, or
  the user's app access is removed** is set to Delete. Set it to **Suspend** as
  described in [Create the SCIM app](#create-the-scim-app-in-onelogin), then
  retry each affected user. The retry deactivates them.
- **"This app uses a connector Obot does not support."** Create a new app with
  the **SCIM Provisioner with SAML (SCIM v2 Core)** connector, and move access
  to it.
- **"externalId" or "userName" errors.** Fix the template or the SCIM Username
  parameter as the error says, then retry.
- **"a user with userName … already exists" when restoring access.** The
  template's `userName` line was changed while **SCIM Username** stayed on
  Username. Set **SCIM Username** to **OneLogin ID** and restore the template's
  `userName` line to `{$parameters.scimusername}`, then retry the user. The
  retry finds the existing user and reactivates them.

### Rotate the SCIM token

The SCIM token expires after one year. Obot shows a banner when it's about to
expire.

1. On the Obot SCIM page, select **Rotate token** and copy the new token.
2. In the SCIM app in OneLogin, update **SCIM Bearer Token** on the
   **Configuration** tab.
3. Optionally, select **Revoke previous token** in Obot. Otherwise, the previous
   token stops working after one day, or on its original expiration date if
   that comes first.

If the token has leaked, select **Revoke current token** instead. Both the
current and previous tokens stop working immediately, so provisioning fails
until you update the token in OneLogin.

## Stop using SCIM

Deconfiguring the OneLogin auth provider, or switching to another auth provider,
removes the SCIM setup:

- OneLogin groups, their memberships, and their role assignments are deleted,
  and the groups are removed from access policies.
- Users are kept. Users that SCIM disabled stay disabled until an administrator
  enables them on the **Users** page.
- Requests from the SCIM app fail. Turn off provisioning in the SCIM app in
  OneLogin, or delete the app.

To go back to reading roles from the OneLogin API, configure OneLogin again with
**API Client ID** and **API Client Secret**. To use SCIM again later, start over
with a new token.
