package types

type Catalog struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type CatalogList List[Catalog]
