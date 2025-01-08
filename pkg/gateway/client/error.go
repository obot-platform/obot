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
