package apiproxy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMatchOperationsStrictUniquePath(t *testing.T) {
	operations := []Operation{
		{OperationKey: "GET /items/{id}", Method: "GET", PathTemplate: "/items/{id}"},
		{OperationKey: "GET /items/list", Method: "GET", PathTemplate: "/items/list"},
	}

	match, matchError := MatchOperations(operations, "GET", "/it%65ms/a%252Fb")

	require.Empty(t, matchError)
	require.Equal(t, "GET /items/{id}", match.Operation.OperationKey)
	require.Equal(t, map[string]string{"id": "a%252Fb"}, match.Bindings)
}

func TestMatchOperationsPrefersStaticAndRejectsAmbiguity(t *testing.T) {
	operations := []Operation{
		{OperationKey: "GET /items/{id}", Method: "GET", PathTemplate: "/items/{id}"},
		{OperationKey: "GET /items/list", Method: "GET", PathTemplate: "/items/list"},
		{OperationKey: "GET /{group}/values", Method: "GET", PathTemplate: "/{group}/values"},
	}

	match, matchError := MatchOperations(operations, "GET", "/items/list")
	require.Empty(t, matchError)
	require.Equal(t, "GET /items/list", match.Operation.OperationKey)

	ambiguous := []Operation{
		{OperationKey: "GET /items/a", Method: "GET", PathTemplate: "/items/{first}"},
		{OperationKey: "GET /items/b", Method: "GET", PathTemplate: "/items/{second}"},
	}
	_, matchError = MatchOperations(ambiguous, "GET", "/items/list")
	require.Equal(t, "ambiguous_operation", matchError)
}

func TestMatchOperationsRejectsInvalidPaths(t *testing.T) {
	for _, rawPath := range []string{"", "/items/", "/items//a", "/items/%2E", "/items/%2E%2E", "/items/%zz"} {
		_, matchError := MatchOperations([]Operation{{Method: "GET", PathTemplate: "/items/{id}"}}, "GET", rawPath)
		require.Equal(t, "invalid_path", matchError)
	}
}
