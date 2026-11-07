---
displayed_sidebar: sidebar
title: "Provision users and groups from Microsoft Entra ID with SCIM"
---

# Provision users and groups from Microsoft Entra ID with SCIM

With SCIM, Microsoft Entra ID sends Obot your users, groups, and group
memberships, and tells Obot when a user should lose access. You decide in Entra
exactly which users and groups Obot can see, and Obot no longer needs the
Microsoft Graph application permissions that let it read your whole directory.

Users still sign in through the Entra app described in
[Entra](./auth-providers.md#entra). Entra's provisioning service sends user and
group changes to Obot from an enterprise app. That can be the same app users
sign in with, if it was created as an enterprise app. See [Set up provisioning
in the enterprise app](#set-up-provisioning-in-the-enterprise-app).

Only an Obot Owner can set up SCIM.

:::note Entra must be able to reach Obot
Entra sends SCIM requests to Obot from Microsoft's own servers, so
`https://<your-obot-url>/scim/v2` and all routes under it must be reachable from
the internet. If Obot is on a private network or behind a firewall, allow Entra
to reach those routes, or provisioning will fail.
:::

## How SCIM changes user management

- **Entra controls who has access.** Assigning a user to the enterprise app,
  directly or through an assigned group, gives them an Obot account. Unassigning
  them, blocking their sign-in, or soft-deleting them deactivates them in Obot:
  they are signed out, and their API keys and other credentials stop working.
  Their account and data are kept, and restoring them in Entra reactivates the
  same account.
- **Deleting an assigned user permanently in Entra deletes them in Obot.**
  Their Obot account is deleted along with their agents, MCP servers, vMCPs,
  and API keys.
  Entra also deletes a soft-deleted user permanently after 30 days. See [Keep
  deleted users' data](#keep-deleted-users-data) to prevent this.
- **Only groups you assign to the enterprise app appear in Obot.** Group IDs in
  Obot are the same as when Obot read groups from Microsoft Graph, so existing
  roles and policies keep working.
- **Changes reach Obot on Entra's schedule.** Entra sends changes in cycles that
  run every 20 to 40 minutes. **Provision on demand** sends a single user or
  group right away.
- **Enforcing SCIM limits sign-in to provisioned users.** Until you enforce
  SCIM, anyone who can sign in through the Entra app can still use Obot.
- **SCIM stays on.** You can't switch Entra back to reading groups from
  Microsoft Graph without removing the Entra auth provider. See [Stop using
  SCIM](#stop-using-scim).

:::warning Unassigning a group removes its access
When you unassign a group from the enterprise app, Entra tells Obot to delete
it, and its members lose the access it grants.

- **Before you enforce SCIM,** Obot empties the group and keeps it. If roles,
  policies, or vMCP profiles use it, Enforce is blocked until you assign it
  again or remove those references. Otherwise, Enforce deletes it.
- **Once SCIM is enforced,** Obot deletes the group, its role assignments, and
  its place in access policies. Assigning it again brings back the same group
  and its members, but not its role assignments or access policies. vMCP
  profiles that still name the group grant access again, but what members lost
  when the group was deleted, described below, doesn't come back.

When a user loses a group, Obot also removes what depended on the access that
only that group, or a role it gave them, granted:

- their connections to shared vMCPs whose profiles no longer include them;
- tools they selected on a shared vMCP that their remaining profiles don't
  grant;
- the entries in their personal vMCPs that their MCP access policies no longer
  let them use. A personal vMCP left with no entries is deleted.
:::

## Choose your setup path

- If you haven't configured Entra in Obot yet, follow [Set up Entra with
  SCIM](#set-up-entra-with-scim).
- If Entra is already your auth provider, reading groups from Microsoft Graph,
  follow [Move an existing Entra setup to
  SCIM](#move-an-existing-entra-setup-to-scim).

## Set up Entra with SCIM

### Step 1: Configure Entra with SCIM

1. Create the Entra app as described in [Entra](./auth-providers.md#entra). Create
   it under **Enterprise applications**, so that it can also provision users,
   and skip the Microsoft Graph application permissions.
2. In Obot, go to **Identity & Access > Auth Providers** and configure
   **Microsoft Entra**.
3. Fill in **Client ID**, **Client Secret**, **Tenant ID**, and **Allowed E-Mail
   Domains**. Leave **Directory integration** set to **SCIM**.
4. Sign in to Obot through Entra as the account that will be the Owner.

### Step 2: Generate the SCIM token

1. Go to **Identity & Access > Auth Providers > SCIM**. This page walks through
   the rest of the setup in steps.
2. Select **Generate token**.
3. Copy the **Base URL** and the token. The token is shown only once.

### Step 3: Set up provisioning in Entra

Follow [Set up provisioning in the enterprise
app](#set-up-provisioning-in-the-enterprise-app) using the base URL and token
from step 2. When you assign users and groups, include your own account.

Assigned groups appear in Obot, where you can use them in roles and access
policies like any other group.

### Step 4: Enforce SCIM

1. Return to **Identity & Access > Auth Providers > SCIM**.
2. Review the users Entra has provisioned. Users listed under **Not provisioned**
   will be disabled when you enforce. Assign them to the enterprise app in Entra
   if they should keep access.
3. Select **Enforce SCIM**.

To enforce, you must have signed in through Entra, and Entra must have
provisioned your own account and not deactivated it.

From now on, only users that Entra has provisioned can sign in. Enforcing can't
be undone.

## Move an existing Entra setup to SCIM

Moving to SCIM keeps your existing users, groups, roles, and policies. Users
keep their accounts and data. Groups keep their IDs, roles, and policy
assignments, because Entra sends each group's object ID and Obot matches it to
the group it already has. While Entra adds members back to each group, though,
some vMCP connections and selections can be lost. See [Step 1: Review the move
in Obot](#step-1-review-the-move-in-obot) for how to prevent that.

### Step 1: Review the move in Obot

Go to **Identity & Access > Auth Providers > SCIM**. The **Move Microsoft Entra
to SCIM** panel lists anything you need to fix first, and what will change.

- **Users whose object IDs Obot hasn't recorded.** Obot
  matches each Entra user to their Obot account by the user's object ID, which
  Obot records when they sign in. If the panel lists users whose IDs Obot
  hasn't recorded, each of them must sign in to Obot in a browser, or be deleted
  in Obot, before you can enable SCIM. A disabled user must be enabled on the
  **Users** page before they can sign in.
- **Duplicate group names.** If two Entra groups that roles, policies, or
  [vMCP profiles](../mcp-gateway/access.md#virtual-mcps-tools-and-profiles) use
  have the same name, remove the references to all but one, or rename one of
  them in Entra. Obot requires the names of provisioned groups to be unique.
  Obot learns a group's new name the next time one of its members signs in.

Before you enable SCIM, know what changes for group members:

- **Members of each group lose its access until Entra adds them back.** When
  Entra provisions a group, it creates the group empty, then adds its members
  one at a time. Obot replaces the group's members with the ones Entra sends,
  so until Entra's first cycle adds them back, members lose the access that the
  group grants. The more members you assign, the longer the first cycle takes.
  **Provision on demand** doesn't shorten this: it sends a group with only the
  members you select.
- **Some vMCP access is lost for good.** While members are out of a group, Obot
  removes what depended on access that only the group, or a role it gives them,
  granted:
  - their connections to shared vMCPs whose profiles no longer include them;
  - tools they selected on a shared vMCP that their remaining profiles don't
    grant;
  - the entries in their personal vMCPs that their MCP access policies no
    longer let them use. A personal vMCP left with no entries is deleted.

  None of this comes back when Entra adds the members back. To prevent it,
  before you enable SCIM, add those users individually to the
  [vMCP profiles](../mcp-gateway/access.md#virtual-mcps-tools-and-profiles) and
  [MCP access policies](../mcp-gateway/access.md#mcp-access-policies-overview)
  that include them through groups or roles.
- **Nested groups no longer grant access.** When Obot read groups from
  Microsoft Graph, it included each user's nested group memberships. Entra
  provisions only the direct members of assigned groups. Access that came
  through a nested group ends once Entra provisions the group. Assign the
  nested groups too, or add their members to the parent group directly.

### Step 2: Enable SCIM

1. Select **Enable SCIM** and confirm.
2. Copy the **Base URL** and the token. The token is shown only once.

When you enable SCIM:

- Everyone can still sign in.
- Obot stops reading groups from Microsoft Graph. Until Entra provisions a
  group, it keeps its current members, including anyone you remove from it in
  Entra.

Enabling SCIM can't be undone.

### Step 3: Set up provisioning in Entra

Follow [Set up provisioning in the enterprise
app](#set-up-provisioning-in-the-enterprise-app) using the base URL and token
from step 2. When you assign users and groups, assign:

- the same users and groups that can sign in to Obot, including your own
  account;
- every group listed under **Referenced groups not pushed yet** on the Obot SCIM
  page. Each group links to its page in the Microsoft Entra admin center.

If you no longer need a listed group, remove it from the roles, policies, and
vMCP profiles that use it instead.

After you start provisioning, wait for Entra's first cycle to finish. The
enterprise app's **Provisioning** page shows its progress.

### Step 4: Enforce SCIM

1. Return to **Identity & Access > Auth Providers > SCIM** and review the
   **Enforce** step. It lists anything that still blocks enforcing, such as
   groups that haven't been provisioned.
2. Review the users under **Not provisioned**. They will be disabled when you
   enforce. Assign them to the enterprise app in Entra if they should keep
   access. Disabled users keep their accounts and data, and assigning them
   later re-enables them.
3. Select **Enforce SCIM**.

To enforce, you must have signed in through Entra, and Entra must have
provisioned your account and not deactivated it. This makes sure you can still
sign in afterward.

### Step 5: Remove the application permissions (optional)

Obot no longer reads users and groups from Microsoft Graph. In the app
registration's **API permissions** tab, remove the **application** permissions
`GroupMember.Read.All` and `User.Read.All`. Keep the **delegated** permissions:
Obot still uses them when users sign in.

## Set up provisioning in the enterprise app

You'll need the SCIM base URL and token from Obot.

Only an app created under **Enterprise applications** can be used for
provisioning. If you created Obot's sign-in app under **App registrations**,
create a separate app for provisioning instead: in the Microsoft Entra admin
center, go to **Entra ID > Enterprise apps > New application > Create your own
application**, select **Integrate any other application you don't find in the
gallery (Non-gallery)**, and select **Create**. Users still sign in through the
original app. If that app has **Assignment required** set to **Yes**, assign
the same users and groups to both apps.

:::warning Finish this setup before you start provisioning
With Entra's default attribute mapping, Obot can't match users to their
sign-ins and refuses them. Finish every step below before you select **Start
provisioning** or **Provision on demand**.
:::

1. In the Microsoft Entra admin center, go to **Entra ID > Enterprise apps** and
   open the app.
2. Under **Manage**, select **Provisioning**, then **New configuration**.
3. Set **Tenant URL** to the base URL from Obot, and **Secret Token** to the
   token. Select **Test Connection**, then **Create**.
4. Under **Attribute mapping**, select **Provision Microsoft Entra ID Users**.
   Edit the mapping whose target attribute is `externalId`, change its source
   attribute from `mailNickname` to `objectId`, and save. Obot matches each user
   to their Entra sign-in by object ID.
5. Decide what happens when users are deleted in Entra. See [Keep deleted users'
   data](#keep-deleted-users-data).
6. Under **Settings**, set **Scope** to **Sync only assigned users and
   groups**.
7. Under **Manage > Users and groups**, assign the users and groups who should
   be able to use Obot.
8. On the **Overview** page, select **Start provisioning**.

On the Obot SCIM page, the **SCIM app** step is marked done once Obot receives
its first request from Entra, which **Test Connection** already sends.

Entra's first cycle provisions every assigned user and group. It can take a
while, since Entra adds each group's members one at a time.

### Keep deleted users' data

By default, Entra tells Obot to delete a user when the user is permanently
deleted in Entra, and when Entra permanently deletes a soft-deleted user after 30
days. Obot then deletes the user's account, agents, MCP servers, vMCPs, and API
keys.

To keep deleted users' data in Obot instead, edit the **Provision Microsoft
Entra ID Users** mapping, clear **Delete** under **Target object actions**, and
save. Entra then sends nothing when it deletes a user. Soft-deleted users stay
deactivated in Obot, and you can delete them from the **Users** page in Obot.
If provisioning is already running, saving the mapping starts a new initial
cycle.

Obot refuses to delete its last Owner, or its last Admin when there is no other
Owner. Entra's request fails until you give another user that role, and Entra
retries it later, possibly hours later. This doesn't protect the account from
being deactivated: unassigning, blocking, or soft-deleting the last Owner in
Entra still signs them out of Obot.

## Manage users and groups with SCIM

| Desired Action | Do this in Entra |
| --- | --- |
| Give someone access | Assign them, or a group they're in, to the enterprise app. |
| Remove someone's access | Unassign them, block their sign-in, or soft-delete them. A permanent delete, including Entra's own 30 days after a soft delete, also [deletes their Obot data](#keep-deleted-users-data). |
| Add a group to Obot | Assign it to the enterprise app. |
| Change group membership or rename a group | Make the change in Entra. Obot is updated in the next cycle. |

Entra treats a user who is in an assigned group as assigned. Unassigning that
user directly doesn't remove their access. Remove them from the group too.

A provisioned user's status in Obot is managed by Entra and can't be changed on
the **Users** page. To delete a user's Obot account and data, either delete
them permanently in Entra while they are still assigned to the enterprise app,
or remove their access in Entra and then delete them from the **Users** page in
Obot. Once a user is unassigned, Entra stops managing them and doesn't tell
Obot when they are deleted.

### What Entra sends for each change

| Change in Entra | Result in Obot |
| --- | --- |
| Unassign a user, block their sign-in, or soft-delete them | The user is deactivated. A soft delete also removes them from their groups. |
| Unblock or restore a user | The user is reactivated. A restore also adds them back to their groups. |
| Delete an assigned user permanently, or 30 days pass after a soft delete | The user and their data are deleted, unless you [cleared **Delete**](#keep-deleted-users-data). |
| Rename a group | The group is renamed and keeps its ID, members, and grants. |
| Unassign a group | Before you enforce, the group is emptied and kept. Once SCIM is enforced, it is deleted with its role assignments and access policies. |
| Assign a group again | The same group comes back, with its members. Role assignments and access policies it lost when it was deleted don't come back. vMCP profiles that still name it grant access again, but members' lost vMCP connections and selections aren't restored. |

### Limitations

- **Group names must be unique** among the groups assigned to the enterprise
  app. Entra can have several groups with the same name, but Obot refuses to
  provision a second one.
- **Nested groups aren't flattened.** Entra provisions only the direct members
  of assigned groups.
- **Changes are not immediate.** Entra's cycles run every 20 to 40 minutes.

### Restart provisioning after an outage

If Entra's requests fail for two cycles in a row, for example because Obot is
unreachable or its Entra auth provider isn't configured, Entra quarantines the
provisioning job. That can take as little as about 20 minutes. A quarantined
job can stay quarantined for hours, even once Obot is back.

On the enterprise app's **Provisioning** page, select **Restart provisioning**.
Entra's first request follows about 20 minutes later.

The Obot SCIM page warns you when Entra hasn't sent a request for about 90
minutes, longer than its usual cycle.

### Retry failed changes

Entra retries failed changes, but not always in the next cycle. A retry can
come hours later. Its **Provisioning logs** show each failure with Obot's error
message, which says what to fix.

Common errors:

- **"externalId must be the user's Entra object ID."** The user mapping still
  sends `mailNickname`. Change it as described in [Set up provisioning in the
  enterprise app](#set-up-provisioning-in-the-enterprise-app). Saving the
  mapping starts a new initial cycle, which provisions the refused users.
- **"The externalId mapping changed."** The user mapping sends a value other
  than the object ID that Obot matched the user by. Map `objectId` to
  `externalId` again.
- **"this user is Obot's last owner" or "last admin".** Give another user that
  role in Obot. Entra retries the delete later.
- **"a group named … already exists".** Two assigned groups have the same name.
  Rename one of them in Entra.

### Rotate the SCIM token

The SCIM token expires after one year. Obot shows a banner when it's about to
expire.

1. On the Obot SCIM page, select **Rotate token** and copy the new token.
2. On the enterprise app's **Provisioning** page, update **Secret Token** under
   **Admin Credentials**, select **Test Connection**, and save.
3. Optionally, select **Revoke previous token** in Obot. Otherwise, the previous
   token stops working after one day, or on its original expiration date if
   that comes first.

If the token has leaked, select **Revoke current token** instead. Both the
current and previous tokens stop working immediately, so provisioning fails
until you update the token in Entra. If Entra quarantined the job in the
meantime, select **Restart provisioning** after updating it.

## Stop using SCIM

Switching to another auth provider removes the SCIM setup:

- Entra groups, their memberships, and their role assignments are deleted, and
  the groups are removed from access policies.
- Users are kept. Users that SCIM disabled stay disabled until an administrator
  enables them on the **Users** page.
- Requests from Entra fail. Stop provisioning on the enterprise app's
  **Provisioning** page.

To go back to reading groups from Microsoft Graph, temporarily switch to the
Local auth provider, then configure Entra again with **Directory integration**
set to **Microsoft Graph**, and grant the app registration the application
permissions described in [Entra](./auth-providers.md#entra). To use SCIM again
later, start over with a new token.
