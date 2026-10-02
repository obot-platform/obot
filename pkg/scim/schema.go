package scim

import (
	"slices"
	"strings"
)

const (
	userSchema                  = "urn:ietf:params:scim:schemas:core:2.0:User"
	groupSchema                 = "urn:ietf:params:scim:schemas:core:2.0:Group"
	listResponseSchema          = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	patchOpSchema               = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
	errorSchema                 = "urn:ietf:params:scim:api:messages:2.0:Error"
	serviceProviderConfigSchema = "urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"
	resourceTypeSchema          = "urn:ietf:params:scim:schemas:core:2.0:ResourceType"
	schemaSchema                = "urn:ietf:params:scim:schemas:core:2.0:Schema"

	typeString    = "string"
	typeBoolean   = "boolean"
	typeComplex   = "complex"
	typeReference = "reference"
	typeDateTime  = "dateTime"

	mutabilityReadOnly  = "readOnly"
	mutabilityReadWrite = "readWrite"
	mutabilityImmutable = "immutable"

	returnedAlways  = "always"
	returnedDefault = "default"

	uniquenessNone   = "none"
	uniquenessServer = "server"
)

var (
	userResourceSchema = &resourceSchema{
		ID:          userSchema,
		Name:        "User",
		Description: "User Account",
		Attributes: []*attribute{
			stringAttribute("userName", "Unique identifier for the User, typically used by the user to directly authenticate to the service provider.", func(a *attribute) {
				a.Required = true
				a.Uniqueness = uniquenessServer
			}),
			{
				Name:        "name",
				Type:        typeComplex,
				Description: "The components of the user's real name.",
				Mutability:  mutabilityReadWrite,
				Returned:    returnedDefault,
				Uniqueness:  uniquenessNone,
				SubAttributes: []*attribute{
					stringAttribute("formatted", "The full name, including all middle names, titles, and suffixes as appropriate, formatted for display."),
					stringAttribute("familyName", "The family name of the User."),
					stringAttribute("givenName", "The given name of the User."),
					stringAttribute("middleName", "The middle name(s) of the User."),
					stringAttribute("honorificPrefix", "The honorific prefix(es) of the User."),
					stringAttribute("honorificSuffix", "The honorific suffix(es) of the User."),
				},
			},
			stringAttribute("displayName", "The name of the User, suitable for display to end-users."),
			stringAttribute("nickName", "The casual way to address the user in real life."),
			referenceAttribute("profileUrl", "A fully qualified URL pointing to a page representing the User's online profile.", "external"),
			stringAttribute("title", "The user's title, such as \"Vice President\"."),
			stringAttribute("userType", "Used to identify the relationship between the organization and the user."),
			stringAttribute("preferredLanguage", "Indicates the User's preferred written or spoken language."),
			stringAttribute("locale", "Used to indicate the User's default location for purposes of localizing items such as currency, date time format, or numerical representations."),
			stringAttribute("timezone", "The User's time zone in the 'Olson' time zone database format."),
			{
				Name:        "active",
				Type:        typeBoolean,
				Description: "A Boolean value indicating the User's administrative status. An inactive user keeps their account and data, and is denied access.",
				Mutability:  mutabilityReadWrite,
				Returned:    returnedDefault,
				Uniqueness:  uniquenessNone,
				// A user is always active or not, so active can be set but not removed.
				unremovable: true,
			},
			multiValuedAttribute("emails", "Email addresses for the user."),
			multiValuedAttribute("phoneNumbers", "Phone numbers for the User."),
			{
				Name:        "groups",
				Type:        typeComplex,
				MultiValued: true,
				Description: "A list of groups to which the user belongs, either through direct membership or through nested groups. Read-only: group membership changes are made through the Group resource.",
				Mutability:  mutabilityReadOnly,
				Returned:    returnedDefault,
				Uniqueness:  uniquenessNone,
				SubAttributes: []*attribute{
					stringAttribute("value", "The identifier of the User's group.", readOnly),
					referenceAttribute("$ref", "The URI of the corresponding 'Group' resource to which the user belongs.", "Group", readOnly),
					stringAttribute("display", "A human-readable name, primarily used for display purposes.", readOnly),
					stringAttribute("type", "A label indicating the attribute's function.", readOnly, func(a *attribute) {
						a.CanonicalValues = []string{"direct"}
					}),
				},
			},
		},
		// Users sign in through the identity provider, so Obot never stores a password, such as the placeholder that
		// Okta sends.
		ignored: []string{"password"},
	}

	groupResourceSchema = &resourceSchema{
		ID:          groupSchema,
		Name:        "Group",
		Description: "Group",
		Attributes: []*attribute{
			stringAttribute("displayName", "A human-readable name for the Group.", func(a *attribute) {
				a.Required = true
				a.Uniqueness = uniquenessServer
			}),
			{
				Name:        "members",
				Type:        typeComplex,
				MultiValued: true,
				Description: "A list of members of the Group. Only users are supported.",
				Mutability:  mutabilityReadWrite,
				Returned:    returnedDefault,
				Uniqueness:  uniquenessNone,
				SubAttributes: []*attribute{
					stringAttribute("value", "Identifier of the member of this Group.", func(a *attribute) {
						a.Required = true
						a.Mutability = mutabilityImmutable
					}),
					referenceAttribute("$ref", "The URI corresponding to a SCIM resource that is a member of this Group.", "User", func(a *attribute) {
						a.Mutability = mutabilityImmutable
					}),
					stringAttribute("display", "A human-readable name, primarily used for display purposes.", readOnly),
					stringAttribute("type", "A label indicating the type of resource.", func(a *attribute) {
						a.Mutability = mutabilityImmutable
						a.CanonicalValues = []string{"User"}
					}),
				},
			},
		},
	}

	// commonAttributes are the attributes every resource has. They are not part of any schema.
	commonAttributes = []*attribute{
		stringAttribute("id", "", readOnly, func(a *attribute) {
			a.CaseExact = true
			a.Returned = returnedAlways
			a.Uniqueness = uniquenessServer
		}),
		stringAttribute("externalId", "", func(a *attribute) {
			a.CaseExact = true
		}),
		{
			Name:       "meta",
			Type:       typeComplex,
			Mutability: mutabilityReadOnly,
			Returned:   returnedDefault,
			Uniqueness: uniquenessNone,
			SubAttributes: []*attribute{
				stringAttribute("resourceType", "", readOnly),
				{
					Name:       "created",
					Type:       typeDateTime,
					Mutability: mutabilityReadOnly,
					Returned:   returnedDefault,
					Uniqueness: uniquenessNone,
				},
				{
					Name:       "lastModified",
					Type:       typeDateTime,
					Mutability: mutabilityReadOnly,
					Returned:   returnedDefault,
					Uniqueness: uniquenessNone,
				},
				referenceAttribute("location", "", "uri", readOnly),
				stringAttribute("version", "", readOnly),
			},
		},
		{
			Name:        "schemas",
			Type:        typeReference,
			MultiValued: true,
			Mutability:  mutabilityReadOnly,
			Returned:    returnedAlways,
			Uniqueness:  uniquenessNone,
		},
	}
)

// resourceSchema describes a SCIM resource's schema. It drives attribute name resolution, PATCH, and discovery.
type resourceSchema struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	Attributes  []*attribute `json:"attributes"`
	// ignored names attributes that clients send but the server does not support. Requests may set them, and they
	// are dropped. They are not advertised.
	ignored []string
}

// attribute describes one SCIM attribute, as RFC 7643 section 7 defines it.
type attribute struct {
	Name            string       `json:"name"`
	Type            string       `json:"type"`
	MultiValued     bool         `json:"multiValued"`
	Description     string       `json:"description,omitempty"`
	Required        bool         `json:"required"`
	CaseExact       bool         `json:"caseExact"`
	Mutability      string       `json:"mutability"`
	Returned        string       `json:"returned"`
	Uniqueness      string       `json:"uniqueness"`
	CanonicalValues []string     `json:"canonicalValues,omitempty"`
	ReferenceTypes  []string     `json:"referenceTypes,omitempty"`
	SubAttributes   []*attribute `json:"subAttributes,omitempty"`
	// unremovable is set for an attribute that a PATCH cannot remove although it is not required.
	unremovable bool
}

func stringAttribute(name, description string, opts ...func(*attribute)) *attribute {
	a := &attribute{
		Name:        name,
		Type:        typeString,
		Description: description,
		Mutability:  mutabilityReadWrite,
		Returned:    returnedDefault,
		Uniqueness:  uniquenessNone,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// referenceAttribute returns a reference attribute, which is case exact, as RFC 7643 section 2.3.7 says.
func referenceAttribute(name, description, referenceType string, opts ...func(*attribute)) *attribute {
	a := stringAttribute(name, description, opts...)
	a.Type = typeReference
	a.CaseExact = true
	a.ReferenceTypes = []string{referenceType}
	return a
}

// multiValuedAttribute returns a multi-valued attribute with the standard value, display, type, and primary
// sub-attributes, like emails.
func multiValuedAttribute(name, description string) *attribute {
	return &attribute{
		Name:        name,
		Type:        typeComplex,
		MultiValued: true,
		Description: description,
		Mutability:  mutabilityReadWrite,
		Returned:    returnedDefault,
		Uniqueness:  uniquenessNone,
		SubAttributes: []*attribute{
			stringAttribute("value", "The attribute's value."),
			stringAttribute("display", "A human-readable name, primarily used for display purposes."),
			stringAttribute("type", "A label indicating the attribute's function, such as \"work\" or \"home\"."),
			{
				Name:        "primary",
				Type:        typeBoolean,
				Description: "A Boolean value indicating the 'primary' or preferred attribute value for this attribute.",
				Mutability:  mutabilityReadWrite,
				Returned:    returnedDefault,
				Uniqueness:  uniquenessNone,
			},
		},
	}
}

func readOnly(a *attribute) {
	a.Mutability = mutabilityReadOnly
}

// attribute returns the schema or common attribute with the given name, compared case-insensitively.
func (s *resourceSchema) attribute(name string) *attribute {
	for _, a := range s.Attributes {
		if strings.EqualFold(a.Name, name) {
			return a
		}
	}
	for _, a := range commonAttributes {
		if strings.EqualFold(a.Name, name) {
			return a
		}
	}
	return nil
}

// ignores reports whether the attribute with the given name is one that the schema accepts and drops.
func (s *resourceSchema) ignores(name string) bool {
	return slices.ContainsFunc(s.ignored, func(ignored string) bool { return strings.EqualFold(ignored, name) })
}

// subAttribute returns the sub-attribute with the given name, compared case-insensitively.
func (a *attribute) subAttribute(name string) *attribute {
	for _, sub := range a.SubAttributes {
		if strings.EqualFold(sub.Name, name) {
			return sub
		}
	}
	return nil
}
