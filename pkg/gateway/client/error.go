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

type ExplicitAdminError struct {
	email string
}

func (e *ExplicitAdminError) Error() string {
	return e.email + " has been marked explicitly as an admin"
}
