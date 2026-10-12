---
displayed_sidebar: sidebar
title: "Provision users and groups from Okta with SCIM"
---

# Provision users and groups from Okta with SCIM

With SCIM, Okta sends Obot your users, groups, and group memberships, and tells
Obot when a user should lose access. You decide in Okta exactly which users and
groups Obot can see, and Obot no longer needs read access to your Okta
directory.

Users still sign in through the Okta OIDC app described in
[Okta](./auth-providers.md#okta). SCIM uses a second Okta app that only sends
user and group changes to Obot. It is never used to sign in.

SCIM is currently available for Okta only. Only an Owner can set it up.

:::note Okta must be able to reach Obot
Okta sends SCIM requests to Obot from Okta's own servers, so
`https://<your-obot-url>/scim/v2` and all routes under it must be reachable from
Okta. If Obot is on a private network or behind a firewall, allow Okta to reach
those routes, or provisioning will fail.
:::

## How SCIM changes user management

- **Okta controls who has access.** Assigning a user to the SCIM app in Okta
  gives them an Obot account. Removing the assignment deactivates them in Obot:
  they are signed out, and their API keys and other credentials stop working.
  Their account and data are kept, and assigning them again restores access.
- **Only groups you push from Okta appear in Obot.** Group membership changes in
  Okta reach Obot right away, instead of when users next sign in.
- **Enforcing SCIM limits sign-in to provisioned users.** Until you enforce
  SCIM, anyone who can sign in through the Okta OIDC app can still use Obot.
- **SCIM stays on.** You can't switch Okta back to fetching groups at sign-in
  without removing the Okta auth provider. See [Stop using
  SCIM](#stop-using-scim).

:::warning Suspending a user in Okta does not remove their access
Okta doesn't notify Obot when a user is suspended. To remove someone's access to
Obot, unassign them from the SCIM app in Okta.
:::

## Choose your setup path

- If you haven't configured Okta in Obot yet, follow [Set up Okta with
  SCIM](#set-up-okta-with-scim).
- If Okta is already your auth provider, configured with an API Services app,
  follow [Move an existing Okta setup to
  SCIM](#move-an-existing-okta-setup-to-scim).

## Set up Okta with SCIM

### Step 1: Configure Okta without an API Services app

1. Create the Okta OIDC app as described in [Okta](./auth-providers.md#okta).
   Skip the API Services app.
2. In Obot, go to **Identity & Access > Auth Providers** and configure **Okta**.
3. Fill in **Client ID**, **Client Secret**, **Org URL**, and **Allowed E-Mail
   Domains**. Leave **API Services Client ID** and **API Services Private Key**
   empty. Leaving them empty tells Obot to use SCIM.
4. Sign in to Obot through Okta as the account that will be the Owner.

### Step 2: Generate the SCIM token

1. Go to **Identity & Access > Auth Providers > SCIM**.
2. Select **Generate token**.
3. Copy the **Base URL** and the token. The token is shown only once.

### Step 3: Create the SCIM app in Okta

Follow [Create the SCIM app in Okta](#create-the-scim-app-in-okta) using the
base URL and token from step 2. Make sure your own account is assigned to the
app.

### Step 4: Push groups

In the SCIM app's **Push Groups** tab, push the Okta groups you want to grant
roles and policies to. You can push more groups at any time. See [Push
groups](#push-groups).

Pushed groups appear in Obot, where you can use them in roles and access
policies like any other group.

### Step 5: Enforce SCIM

1. Return to **Identity & Access > Auth Providers > SCIM**.
2. Review the users Okta has provisioned. Users listed under **Not provisioned**
   will be disabled when you enforce. Assign them to the SCIM app in Okta if
   they should keep access.
3. Select **Enforce SCIM**.

From now on, only users that Okta has provisioned can sign in. Enforcing can't
be undone.

## Move an existing Okta setup to SCIM

Moving to SCIM keeps your existing users, groups, roles, and policies. Users
keep their accounts and data, and groups keep their roles and policy assignments
once Okta pushes them.

### Step 1: Review the move in Obot

Go to **Identity & Access > Auth Providers > SCIM**. The **Move Okta to SCIM**
panel lists anything you need to fix first, and what will change.

- **Replace the `Everyone` group.** Okta can't push its built-in `Everyone`
  group. Change any role, policy, or
  [vMCP profile](../mcp-gateway/access.md#virtual-mcps-tools-and-profiles) that
  uses it to **All Obot Users** instead.
- **Resolve duplicate group names.** If two groups that roles, policies, or vMCP
  profiles use have the same name, remove the references to all but one, or
  rename one of them in Okta. Obot matches pushed groups by name, so names must
  be unique.

### Step 2: Enable SCIM

1. Select **Enable SCIM** and confirm.
2. Copy the **Base URL** and the token. The token is shown only once.

When you enable SCIM:

- Everyone can still sign in.
- Obot stops fetching groups from Okta at sign-in. Until you push a group, it
  keeps its current members, including anyone you remove from it in Okta. Push
  your groups soon after enabling.
- Okta groups that no role, policy, or vMCP profile uses are removed from Obot.
  They grant nothing, so no one loses access, and you can push them later if you
  need them.

Enabling SCIM can't be undone.

### Step 3: Create the SCIM app in Okta

Follow [Create the SCIM app in Okta](#create-the-scim-app-in-okta) using the
base URL and token from step 2. Assign it to the same users and groups as your
Obot OIDC app, and make sure your own account is assigned.

### Step 4: Push the groups Obot lists

In the SCIM app's **Push Groups** tab, push every group listed under
**Referenced groups not pushed yet** on the Obot SCIM page, using the exact name
shown there. Each group links to its page in Okta. See [Push
groups](#push-groups).

If a group was renamed in Okta since Obot last saw it, its name in Obot is out
of date. Rename the group in Okta to the name Obot shows, push it, and then
rename it back. Obot picks up the new name.

If you no longer need a listed group, remove it from the roles, policies, and
vMCP profiles that use it instead.

### Step 5: Enforce SCIM

1. Return to **Identity & Access > Auth Providers > SCIM** and review the
   **Enforce** step. It lists anything that still blocks enforcing, such as
   groups that haven't been pushed.
2. Review the users under **Not provisioned**. They will be disabled when you
   enforce. Assign them to the SCIM app in Okta if they should keep access.
   Disabled users keep their accounts and data, and assigning them later
   re-enables them.
3. Select **Enforce SCIM**.

To enforce, you must have signed in through Okta and be assigned to the SCIM
app. This makes sure you can still sign in afterward.

### Step 6: Remove the API Services credentials (optional)

Obot no longer uses the API Services app. Modify the Okta auth provider to clear
**API Services Client ID** and **API Services Private Key**, then delete the API
Services app in Okta.

## Create the SCIM app in Okta

You'll need your Obot URL, and the SCIM base URL and token from Obot.

1. In the Okta Admin Console, go to **Applications and Resources > Applications**
   and select **Create App Integration**.
2. Select **SWA - Secure Web Authentication** and select **Next**. Okta requires
   an app type, but this app is never used to sign in.
3. Configure the app:
   - **App name:** Any name, such as `Obot SCIM`.
   - **App's login page URL:** `https://<your-obot-url>/okta-scim`. This page
     tells anyone who opens the app to sign in through your regular Obot sign-in
     instead, since this app is used only for SCIM provisioning and not for sign-in.
   - **App visibility:** Select **Do not display application icon to users**.
4. Select **Finish**.
5. In the app's **General** tab, under **App Settings**, select **Edit**, set
   **Provisioning** to **SCIM**, and select **Save**.
6. In the **Sign On** tab, under **Credentials Details**, set **Application
   username format** to **Email**.
7. In the **Provisioning** tab, under **Integration**, select **Edit** and set:
   - **SCIM connector base URL:** The base URL from Obot.
   - **Unique identifier field for users:** `userName`
   - **Supported provisioning actions:** **Push New Users**, **Push Profile
     Updates**, and **Push Groups**. Leave the import options unselected.
   - **Authentication Mode:** **HTTP Header**, with the token from Obot as the
     bearer token.
8. Select **Test Connector Configuration**, then **Save**.
9. In the **Provisioning** tab, under **To App**, select **Edit**, enable
   **Create Users**, **Update User Attributes**, and **Deactivate Users**, and
   select **Save**. Leave **Sync Password** off.
10. In the **Assignments** tab, assign the users and groups who should be able
    to use Obot. Okta then provisions those users in Obot.

On the Obot SCIM page, the **SCIM app** step is marked done once Obot receives
its first request from Okta.

### Push groups

1. In the SCIM app, open the **Push Groups** tab.
2. Select **Push Groups > Find groups by name**.
3. Enter the group's name, select it, keep **Push group memberships
   immediately** selected, and select **Save**.

## Manage users and groups with SCIM

| Desired Action | Do this in Okta |
| --- | --- |
| Give someone access | Assign them to the SCIM app. |
| Remove someone's access | Unassign them from the SCIM app. |
| Add a group to Obot | Push it from the SCIM app's **Push Groups** tab. |
| Change group membership or rename a group | Make the change in Okta. Obot is updated automatically. |

A provisioned user's status in Obot is managed by Okta and can't be changed on
the **Users** page. To delete a user's Obot account and data permanently, first
unassign them in Okta, then delete them from the **Users** page in Obot.

### Rotate the SCIM token

The SCIM token expires after one year. Obot shows a banner when it's about to
expire.

1. On the Obot SCIM page, select **Rotate token** and copy the new token.
2. In the SCIM app in Okta, update the token under **Provisioning >
   Integration**.
3. Optionally, select **Revoke previous token** in Obot. Otherwise, the previous
   token stops working after one day, or on its original expiration date if
   that comes first. If you rotate a token that is about to expire, update it in
   Okta right away.

If the token has leaked, select **Revoke current token** instead. Both the
current and previous tokens stop working immediately, so provisioning fails
until you update the token in Okta.

### Retry failed changes

If Obot can't apply a change from Okta, for example because Obot was
unavailable, Okta may not retry it automatically. The **Activity** section of
the Obot SCIM page lists recent failures.

- **Deactivations:** In Okta, go to **Dashboard > Tasks > Application accounts
  need deprovisioning** and select **Retry Selected**. Don't select **Mark
  Selected Complete**, which closes the task without deactivating the user in
  Obot.
- **Group changes:** In the SCIM app's **Push Groups** tab, select **Retry All
  Groups**.

## Stop using SCIM

Deconfiguring the Okta auth provider, or switching to another auth provider,
removes the SCIM setup:

- Okta groups, their memberships, and their role assignments are deleted, and
  the groups are removed from access policies.
- Users are kept. Users that SCIM disabled stay disabled until an administrator
  enables them on the **Users** page.
- Requests from the SCIM app fail. Turn off provisioning in the SCIM app in
  Okta, or delete the app.

To go back to fetching groups at sign-in, configure Okta again with the **API
Services Client ID** and **API Services Private Key**. To use SCIM again later,
start over with a new token.
