/**
 * For the auth-providers route, we want to test the following test case:
 *
 * User can click Configure for an auth provider, fill out the form, and calls the mocked AdminService.configureAuthProvider
 * -- need two cases here: if the user is a bootstrap, they should see the owner hand off dialog
 * -- otherwise, if they not, they shouldn't see the handoff and the auth provider should shown as configured
 *
 * If user clicks Configure on an auth provider with a missing entitlement, they should see license required and clicking Configure opens the license info dialog.
 */
