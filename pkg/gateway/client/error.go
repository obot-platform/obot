package client

type LastAdminError struct{}

func (e *LastAdminError) Error() string {
	return "last admin"
}

type AlreadyExistsError struct {
	name string
}

func (e *AlreadyExistsError) Error() string {
	return e.name + " already exists"
}

type ExplicitRoleError struct {
	email string
}

func (e *ExplicitRoleError) Error() string {
	return e.email + " has a role that was explicitly set"
}
