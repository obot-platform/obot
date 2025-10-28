package types

type Identity struct {
	Metadata
	AuthProviderName      string `json:"authProviderName"`
	AuthProviderNamespace string `json:"authProviderNamespace"`
	ProviderUsername      string `json:"providerUsername"`
	ProviderUserID        string `json:"providerUserID"`
	Email                 string `json:"email"`
	UserID                uint   `json:"userID"`
	IconURL               string `json:"iconURL"`
}

type IdentityList List[Identity]
