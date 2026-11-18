package client

type LastAdminError struct{}

type LastOwnerError struct{}

func (e *LastAdminError) Error() string {
	return "last admin"
}

func (e *LastOwnerError) Error() string {
	return "last owner"
}
