// Package authz centralizes the access-rule vocabulary shared by the schema
// migrations and any custom routes. Two identity classes exist:
//
//   - staff (auth collection `staff`): agents and admins, cross-customer.
//   - requesters (auth collection `users`): scoped to their one customer.
//
// Rules reference the identity class via @request.auth.collectionName, the
// same two-collection pattern kiosk and access-control use.
package authz

const (
	// StaffCollection and RequesterCollection name the two auth collections
	// for route-level guards (collection rules use the literals below).
	StaffCollection     = "staff"
	RequesterCollection = "users"

	// StaffRule matches any authenticated staff member (agent or admin).
	StaffRule = "@request.auth.collectionName = 'staff'"

	// AdminRule matches staff with the admin role.
	AdminRule = StaffRule + " && @request.auth.role = 'admin'"

	// RequesterRule matches any authenticated requester.
	RequesterRule = "@request.auth.collectionName = 'users'"
)
