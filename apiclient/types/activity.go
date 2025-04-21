package types

type APIActivity struct {
	UserID string `json:"userId"`
	Date   Time   `json:"date"`
}

type APIActivityList List[APIActivity]
