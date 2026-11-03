package client

type LastAdminError struct{}

type LastOwnerError struct{}

type AlreadyExistsError struct {
	name string
}

func (e *LastAdminError) Error() string {
	return "last admin"
}

func (e *LastOwnerError) Error() string {
	return "last owner"
}

func (e *AlreadyExistsError) Error() string {
	return e.name + " already exists"
}
